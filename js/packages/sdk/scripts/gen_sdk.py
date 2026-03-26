#!/usr/bin/env python3
"""
Generate typed data API client for @surf/sdk from surf CLI endpoint schemas.

Outputs:
  src/data/types.ts       — TypeScript interfaces for all endpoints
  src/data/categories/    — Per-category dataApi methods (server) + React hooks
  src/react/hooks/        — Per-category React Query hooks

Usage:
    # Generate all endpoints
    python scripts/gen_sdk.py

    # Generate specific operations
    python scripts/gen_sdk.py --ops market-price wallet-detail

Requires: surf CLI installed and authenticated (surf login).
"""

import argparse
import re
import subprocess
import sys
from dataclasses import dataclass, field as dc_field
from pathlib import Path
from typing import Optional


# ---------------------------------------------------------------------------
# Data model (from surf-core gen_client.py)
# ---------------------------------------------------------------------------

@dataclass
class SchemaField:
    name: str
    type_str: str
    required: bool
    description: str
    format_str: Optional[str] = None
    default: Optional[str] = None
    enum_values: Optional[list[str]] = None
    min_val: Optional[float] = None
    max_val: Optional[float] = None
    children: list["SchemaField"] = dc_field(default_factory=list)
    is_array: bool = False


@dataclass
class Endpoint:
    name: str          # e.g. 'market-price'
    method: str        # 'GET' or 'POST'
    path: str          # e.g. '/market/price'
    category: str      # e.g. 'market'
    method_name: str   # e.g. 'price'
    description: str
    params: list[SchemaField]
    body_fields: list[SchemaField]
    data_fields: list[SchemaField]
    data_is_array: bool
    pagination: str


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

DOMAIN_PREFIXES = sorted(
    [
        "prediction-market", "polymarket", "kalshi",
        "market", "wallet", "social", "token", "project",
        "fund", "onchain", "news", "exchange", "search", "web",
    ],
    key=len,
    reverse=True,
)


# ---------------------------------------------------------------------------
# Parsing (reused from surf-core gen_client.py)
# ---------------------------------------------------------------------------

_FIELD_RE = re.compile(
    r"^(\s*)"
    r"(--)?(\$?\w[\w-]*)"
    r"(\*)?"
    r":\s+"
    r"(.*)"
)
_TYPE_RE = re.compile(r"^\(([^)]+)\)\s*(.*)?$")
_ENUM_RE = re.compile(r'enum:"([^"]*(?:","[^"]*)*)"')
_DEFAULT_RE = re.compile(r'default:"?([^",\s)]+)"?')
_FORMAT_RE = re.compile(r"format:(\S+)")
_MIN_RE = re.compile(r"min:(\d+)")
_MAX_RE = re.compile(r"max:(\d+)")


def _parse_type_annotation(text: str) -> tuple[SchemaField, str]:
    m = _TYPE_RE.match(text)
    if not m:
        return SchemaField(name="", type_str="any", required=False, description=text.strip()), ""
    inner, desc = m.group(1), (m.group(2) or "").strip()
    parts = inner.split(None, 1)
    type_str = parts[0]
    attrs = parts[1] if len(parts) > 1 else ""
    enum_m = _ENUM_RE.search(attrs)
    default_m = _DEFAULT_RE.search(attrs)
    format_m = _FORMAT_RE.search(attrs)
    min_m = _MIN_RE.search(attrs)
    max_m = _MAX_RE.search(attrs)
    return SchemaField(
        name="", type_str=type_str, required=False, description=desc,
        format_str=format_m.group(1) if format_m else None,
        default=default_m.group(1) if default_m else None,
        enum_values=enum_m.group(1).split('","') if enum_m else None,
        min_val=float(min_m.group(1)) if min_m else None,
        max_val=float(max_m.group(1)) if max_m else None,
    ), desc


def _parse_schema_lines(lines: list[str], start: int = 0) -> tuple[list[SchemaField], int]:
    fields: list[SchemaField] = []
    i = start
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()
        if not stripped or stripped in ("```schema", "```"):
            i += 1
            continue
        if stripped in ("}", "]", "},", "],"):
            return fields, i + 1
        if stripped == "{":
            children, i = _parse_schema_lines(lines, i + 1)
            if children:
                if fields and fields[-1].is_array and not fields[-1].children:
                    fields[-1].children = children
                else:
                    fields.extend(children)
            continue
        if stripped.startswith("<any>"):
            fields.append(SchemaField(name="*", type_str="any", required=False, description="Dynamic keys"))
            i += 1
            continue
        m = _FIELD_RE.match(line)
        if not m:
            i += 1
            continue
        _prefix, _dash, name, req_mark, rest = m.groups()
        if _dash:
            name = name.lstrip("-")
        name = name.replace("-", "_")
        required = req_mark == "*"
        rest = rest.strip()
        if name == "$schema":
            i += 1
            continue
        if rest == "[" or rest.startswith("["):
            field = SchemaField(name=name, type_str="array", required=required, description="", is_array=True)
            field.children, i = _parse_schema_lines(lines, i + 1)
            fields.append(field)
            continue
        if rest == "{" or rest.startswith("{"):
            field = SchemaField(name=name, type_str="object", required=required, description="")
            field.children, i = _parse_schema_lines(lines, i + 1)
            fields.append(field)
            continue
        partial, desc = _parse_type_annotation(rest)
        partial.name = name
        partial.required = required
        if not partial.description:
            partial.description = desc
        fields.append(partial)
        i += 1
    return fields, i


def _extract_schema_block(text: str, header: str) -> list[str]:
    lines = text.split("\n")
    in_block = False
    result = []
    found_header = False
    for line in lines:
        if header in line:
            found_header = True
            continue
        if found_header and "```schema" in line:
            in_block = True
            continue
        if in_block:
            if line.strip() == "```":
                break
            result.append(line)
    return result


def _detect_method(help_text: str) -> str:
    if "## Request Schema" in help_text or "## Input Example" in help_text:
        return "POST"
    return "GET"


def _detect_pagination(params: list[SchemaField], meta_text: str) -> str:
    param_names = {p.name for p in params}
    if "cursor" in param_names and "next_cursor" in meta_text:
        return "cursor"
    if "offset" in param_names and "total" in meta_text:
        return "offset"
    return "none"


def op_to_parts(op: str) -> tuple[str, str]:
    """Convert operation name to (category, method_name): 'market-price' → ('market', 'price')."""
    for prefix in DOMAIN_PREFIXES:
        if op == prefix:
            return prefix, "list"
        if op.startswith(prefix + "-"):
            rest = op[len(prefix) + 1:]
            return prefix, rest.replace("-", "_")
    parts = op.split("-", 1)
    return parts[0], parts[1].replace("-", "_") if len(parts) > 1 else "get"


def op_to_path(op: str) -> str:
    for prefix in DOMAIN_PREFIXES:
        if op == prefix:
            return f"/{prefix}"
        if op.startswith(prefix + "-"):
            rest = op[len(prefix) + 1:]
            return f"/{prefix}/{rest}"
    parts = op.split("-", 1)
    return "/" + "/".join(parts)


def parse_help(op: str, help_text: str) -> Endpoint:
    desc_lines = []
    for line in help_text.split("\n"):
        if line.startswith("##"):
            break
        if line.strip():
            desc_lines.append(line.strip())
    description = " ".join(desc_lines)
    method = _detect_method(help_text)
    params: list[SchemaField] = []
    body_fields: list[SchemaField] = []
    if method == "GET":
        option_lines = _extract_schema_block(help_text, "## Option Schema")
        if option_lines:
            params, _ = _parse_schema_lines(option_lines)
    else:
        request_lines = _extract_schema_block(help_text, "## Request Schema")
        if request_lines:
            body_fields, _ = _parse_schema_lines(request_lines)
    response_lines = _extract_schema_block(help_text, "## Response 200")
    resp_fields: list[SchemaField] = []
    if response_lines:
        resp_fields, _ = _parse_schema_lines(response_lines)
    data_fields: list[SchemaField] = []
    data_is_array = True
    for f in resp_fields:
        if f.name == "data":
            data_is_array = f.is_array
            data_fields = f.children
            break
    meta_text = ""
    for f in resp_fields:
        if f.name == "meta":
            meta_text = " ".join(c.name for c in f.children)
            break
    pagination = _detect_pagination(params, meta_text)
    category, method_name = op_to_parts(op)
    return Endpoint(
        name=op, method=method, path=op_to_path(op),
        category=category, method_name=method_name,
        description=description, params=params, body_fields=body_fields,
        data_fields=data_fields, data_is_array=data_is_array, pagination=pagination,
    )


# ---------------------------------------------------------------------------
# Name helpers
# ---------------------------------------------------------------------------

def _pascal(name: str) -> str:
    return "".join(w.capitalize() for w in re.split(r"[-_]", name))


def _camel(name: str) -> str:
    p = _pascal(name)
    return p[0].lower() + p[1:] if p else ""


def _ts_type(field: SchemaField) -> str:
    if field.enum_values:
        return " | ".join(f"'{v}'" for v in field.enum_values)
    if field.is_array and field.children:
        return f"{_pascal(field.name)}Item[]"
    if field.children:
        return _pascal(field.name)
    m = {"string": "string", "integer": "number", "number": "number",
         "boolean": "boolean", "any": "unknown"}
    return m.get(field.type_str, "unknown")


# ---------------------------------------------------------------------------
# TypeScript generators
# ---------------------------------------------------------------------------

def _gen_ts_interface(name: str, fields: list[SchemaField], lines: list[str],
                      nested: list[tuple[str, list[SchemaField]]]):
    lines.append(f"export interface {name} {{")
    for f in fields:
        if f.name == "*":
            lines.append("  [key: string]: unknown;")
            continue
        opt = "" if f.required else "?"
        if f.description:
            lines.append(f"  /** {f.description} */")
        if f.is_array and f.children:
            child_name = f"{name}{_pascal(f.name)}Item"
            lines.append(f"  {f.name}{opt}: {child_name}[];")
            nested.append((child_name, f.children))
        elif f.children:
            child_name = f"{name}{_pascal(f.name)}"
            lines.append(f"  {f.name}{opt}: {child_name};")
            nested.append((child_name, f.children))
        else:
            lines.append(f"  {f.name}{opt}: {_ts_type(f)};")
    lines.append("}")
    lines.append("")


def generate_types(endpoints: list[Endpoint], out: Path):
    """Generate src/data/types.ts with all interfaces."""
    lines = [
        "// Auto-generated by gen_sdk.py — do not edit.",
        "",
        "export interface ResponseMeta {",
        "  cached?: boolean;",
        "  credits_used?: number;",
        "  total?: number;",
        "  limit?: number;",
        "  offset?: number;",
        "}",
        "",
        "export interface CursorMeta {",
        "  cached?: boolean;",
        "  credits_used?: number;",
        "  has_more?: boolean;",
        "  next_cursor?: string;",
        "  limit?: number;",
        "}",
        "",
        "export interface ApiResponse<T> { data: T[]; meta?: ResponseMeta; }",
        "export interface ApiObjectResponse<T> { data: T; meta?: ResponseMeta; }",
        "export interface ApiCursorResponse<T> { data: T[]; meta?: CursorMeta; }",
        "",
    ]

    for ep in endpoints:
        pascal = _pascal(ep.name)
        item_name = f"{pascal}Item" if ep.data_is_array else f"{pascal}Data"

        nested: list[tuple[str, list[SchemaField]]] = []
        _gen_ts_interface(item_name, ep.data_fields, lines, nested)
        while nested:
            n, f = nested.pop(0)
            _gen_ts_interface(n, f, lines, nested)

        param_fields = ep.params if ep.method == "GET" else ep.body_fields
        if param_fields:
            param_nested: list[tuple[str, list[SchemaField]]] = []
            lines.append(f"export interface {pascal}Params {{")
            for p in param_fields:
                opt = "" if p.required else "?"
                if p.description:
                    doc_parts = [p.description]
                    if p.default:
                        doc_parts.append(f"@default '{p.default}'")
                    lines.append(f"  /** {' — '.join(doc_parts)} */")
                if p.is_array and p.children:
                    child_name = f"{pascal}Params{_pascal(p.name)}Item"
                    lines.append(f"  {p.name}{opt}: {child_name}[];")
                    param_nested.append((child_name, p.children))
                elif p.children:
                    child_name = f"{pascal}Params{_pascal(p.name)}"
                    lines.append(f"  {p.name}{opt}: {child_name};")
                    param_nested.append((child_name, p.children))
                else:
                    lines.append(f"  {p.name}{opt}: {_ts_type(p)};")
            lines.append("}")
            lines.append("")
            # Generate nested param interfaces
            while param_nested:
                n, f = param_nested.pop(0)
                _gen_ts_interface(n, f, lines, param_nested)

    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("\n".join(lines) + "\n")
    print(f"  wrote {out}")


def generate_server_categories(endpoints: list[Endpoint], out_dir: Path):
    """Generate src/data/categories/{category}.ts — typed dataApi methods."""
    by_cat: dict[str, list[Endpoint]] = {}
    for ep in endpoints:
        by_cat.setdefault(ep.category, []).append(ep)

    out_dir.mkdir(parents=True, exist_ok=True)

    # Per-category files
    for cat, eps in sorted(by_cat.items()):
        cat_safe = cat.replace("-", "_")
        lines = [
            "// Auto-generated by gen_sdk.py — do not edit.",
            "",
            "import { get, post } from '../client';",
        ]

        # Collect type imports
        type_imports = []
        for ep in eps:
            pascal = _pascal(ep.name)
            item_name = f"{pascal}Item" if ep.data_is_array else f"{pascal}Data"
            if ep.data_is_array:
                type_imports.append("ApiCursorResponse" if ep.pagination == "cursor" else "ApiResponse")
            else:
                type_imports.append("ApiObjectResponse")
            type_imports.append(item_name)
            param_fields = ep.params if ep.method == "GET" else ep.body_fields
            if param_fields:
                type_imports.append(f"{pascal}Params")
        type_imports = sorted(set(type_imports))
        lines.append(f"import type {{ {', '.join(type_imports)} }} from '../types';")
        lines.append("")

        # Category object
        lines.append(f"export const {cat_safe} = {{")
        for ep in eps:
            pascal = _pascal(ep.name)
            item_name = f"{pascal}Item" if ep.data_is_array else f"{pascal}Data"
            param_fields = ep.params if ep.method == "GET" else ep.body_fields
            has_params = bool(param_fields)
            params_optional = has_params and not any(p.required for p in param_fields)

            if ep.data_is_array:
                ret = f"ApiCursorResponse<{item_name}>" if ep.pagination == "cursor" else f"ApiResponse<{item_name}>"
            else:
                ret = f"ApiObjectResponse<{item_name}>"

            if ep.description:
                lines.append(f"  /** {ep.description} */")

            opt = "?" if params_optional else ""
            if ep.method == "GET":
                if has_params:
                    lines.append(f"  {ep.method_name}: (params{opt}: {pascal}Params): Promise<{ret}> =>")
                    lines.append(f"    get('{ep.path.lstrip('/')}', params as any),")
                else:
                    lines.append(f"  {ep.method_name}: (): Promise<{ret}> =>")
                    lines.append(f"    get('{ep.path.lstrip('/')}'),")
            else:
                if has_params:
                    lines.append(f"  {ep.method_name}: (body{opt}: {pascal}Params): Promise<{ret}> =>")
                    lines.append(f"    post('{ep.path.lstrip('/')}', body),")
                else:
                    lines.append(f"  {ep.method_name}: (): Promise<{ret}> =>")
                    lines.append(f"    post('{ep.path.lstrip('/')}'),")
            lines.append("")

        lines.append("};")
        lines.append("")

        (out_dir / f"{cat_safe}.ts").write_text("\n".join(lines) + "\n")
        print(f"  wrote {out_dir / f'{cat_safe}.ts'}")

    # Index file that assembles dataApi
    index_lines = [
        "// Auto-generated by gen_sdk.py — do not edit.",
        "",
        "import { get, post } from './client';",
    ]
    for cat in sorted(by_cat.keys()):
        cat_safe = cat.replace("-", "_")
        index_lines.append(f"import {{ {cat_safe} }} from './categories/{cat_safe}';")
    index_lines.append("")
    index_lines.append("export const dataApi = {")
    index_lines.append("  /** Escape hatch: raw GET to any endpoint path. */")
    index_lines.append("  get,")
    index_lines.append("  /** Escape hatch: raw POST to any endpoint path. */")
    index_lines.append("  post,")
    for cat in sorted(by_cat.keys()):
        cat_safe = cat.replace("-", "_")
        index_lines.append(f"  {cat_safe},")
    index_lines.append("};")
    index_lines.append("")
    index_lines.append("export type { dataApi as DataApi };")
    index_lines.append("")

    (out_dir.parent / "data-api.ts").write_text("\n".join(index_lines) + "\n")
    print(f"  wrote {out_dir.parent / 'data-api.ts'}")


def generate_react_hooks(endpoints: list[Endpoint], out_dir: Path):
    """Generate src/react/hooks/{category}.ts — React Query hooks."""
    by_cat: dict[str, list[Endpoint]] = {}
    for ep in endpoints:
        by_cat.setdefault(ep.category, []).append(ep)

    out_dir.mkdir(parents=True, exist_ok=True)

    for cat, eps in sorted(by_cat.items()):
        cat_safe = cat.replace("-", "_")
        lines = [
            "// Auto-generated by gen_sdk.py — do not edit.",
            "",
            "import { useQuery, useInfiniteQuery } from '@tanstack/react-query';",
            "import { proxyGet, proxyPost } from '../fetch';",
        ]

        type_imports = []
        for ep in eps:
            pascal = _pascal(ep.name)
            item_name = f"{pascal}Item" if ep.data_is_array else f"{pascal}Data"
            if ep.data_is_array:
                type_imports.append("ApiCursorResponse" if ep.pagination == "cursor" else "ApiResponse")
            else:
                type_imports.append("ApiObjectResponse")
            type_imports.append(item_name)
            param_fields = ep.params if ep.method == "GET" else ep.body_fields
            if param_fields:
                type_imports.append(f"{pascal}Params")
        type_imports = sorted(set(type_imports))
        lines.append(f"import type {{ {', '.join(type_imports)} }} from '../../data/types';")
        lines.append("")

        for ep in eps:
            pascal = _pascal(ep.name)
            item_name = f"{pascal}Item" if ep.data_is_array else f"{pascal}Data"
            param_fields = ep.params if ep.method == "GET" else ep.body_fields
            has_params = bool(param_fields)
            params_optional = has_params and not any(p.required for p in param_fields)
            opt = "?" if params_optional else ""

            if ep.data_is_array:
                ret = f"ApiCursorResponse<{item_name}>" if ep.pagination == "cursor" else f"ApiResponse<{item_name}>"
            else:
                ret = f"ApiObjectResponse<{item_name}>"

            if ep.description:
                lines.append(f"/** {ep.description} */")

            if ep.pagination in ("offset", "cursor"):
                # Infinite query
                params_type = f"{pascal}Params" if has_params else None
                if has_params:
                    omit_field = "offset" if ep.pagination == "offset" else "cursor"
                    lines.append(f"export function useInfinite{pascal}(params{opt}: Omit<{params_type}, '{omit_field}'>) {{")
                else:
                    lines.append(f"export function useInfinite{pascal}() {{")
                lines.append("  return useInfiniteQuery({")
                lines.append(f"    queryKey: ['{ep.name}', params],")
                if ep.pagination == "offset":
                    if has_params:
                        lines.append(f"    queryFn: ({{ pageParam = 0 }}) => proxyGet<{ret}>('{ep.path.lstrip('/')}', {{ ...params!, offset: String(pageParam) }}),")
                    else:
                        lines.append(f"    queryFn: () => proxyGet<{ret}>('{ep.path.lstrip('/')}'),")
                    lines.append("    initialPageParam: 0,")
                    lines.append("    getNextPageParam: (last) => {")
                    lines.append("      const m = last?.meta;")
                    lines.append("      if (!m?.total || !m?.limit) return undefined;")
                    lines.append("      const next = (m.offset ?? 0) + m.limit;")
                    lines.append("      return next < m.total ? next : undefined;")
                    lines.append("    },")
                else:
                    if has_params:
                        lines.append(f"    queryFn: ({{ pageParam }}) => proxyGet<{ret}>('{ep.path.lstrip('/')}', {{ ...params!, cursor: pageParam || undefined }}),")
                    else:
                        lines.append(f"    queryFn: () => proxyGet<{ret}>('{ep.path.lstrip('/')}'),")
                    lines.append("    initialPageParam: '',")
                    lines.append("    getNextPageParam: (last) => last?.meta?.has_more ? last.meta.next_cursor : undefined,")
                lines.append("  });")
                lines.append("}")
            else:
                # Standard query
                if has_params:
                    lines.append(f"export function use{pascal}(params{opt}: {pascal}Params) {{")
                else:
                    lines.append(f"export function use{pascal}() {{")
                lines.append("  return useQuery({")
                if has_params:
                    lines.append(f"    queryKey: ['{ep.name}', params],")
                else:
                    lines.append(f"    queryKey: ['{ep.name}'],")
                if ep.method == "GET":
                    if has_params:
                        lines.append(f"    queryFn: () => proxyGet<{ret}>('{ep.path.lstrip('/')}', params as any),")
                    else:
                        lines.append(f"    queryFn: () => proxyGet<{ret}>('{ep.path.lstrip('/')}'),")
                else:
                    if has_params:
                        lines.append(f"    queryFn: () => proxyPost<{ret}>('{ep.path.lstrip('/')}', params),")
                    else:
                        lines.append(f"    queryFn: () => proxyPost<{ret}>('{ep.path.lstrip('/')}'),")
                lines.append("  });")
                lines.append("}")
            lines.append("")

        (out_dir / f"{cat_safe}.ts").write_text("\n".join(lines) + "\n")
        print(f"  wrote {out_dir / f'{cat_safe}.ts'}")

    # React index with re-exports
    index_lines = [
        "// Auto-generated by gen_sdk.py — do not edit.",
        "",
        "// Manual exports (utilities + bootstrap)",
        "export { cn } from './utils';",
        "export { useToast, toast } from './use-toast';",
        "",
        "// Re-export all hooks",
    ]
    for cat in sorted(by_cat.keys()):
        cat_safe = cat.replace("-", "_")
        index_lines.append(f"export * from './hooks/{cat_safe}';")
    index_lines.append("")
    index_lines.append("// Re-export types")
    index_lines.append("export * from '../data/types';")
    index_lines.append("")

    (out_dir.parent / "index.ts").write_text("\n".join(index_lines) + "\n")
    print(f"  wrote {out_dir.parent / 'index.ts'}")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def get_all_operations() -> list[str]:
    """Get all available operations from surf CLI."""
    result = subprocess.run(
        ["surf", "list-operations"], capture_output=True, text=True,
    )
    if result.returncode != 0:
        print(f"ERROR: surf list-operations failed: {result.stderr}", file=sys.stderr)
        sys.exit(1)
    ops = []
    for line in result.stdout.strip().split("\n"):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        # Format: "  GET    market-price    Description (--params)"
        # Strip HTTP method prefix and description suffix
        parts = line.split()
        if len(parts) >= 2 and parts[0] in ("GET", "POST", "PUT", "PATCH", "DELETE"):
            ops.append(parts[1])
        elif parts:
            ops.append(parts[0])
    return ops


def get_help(op: str) -> str:
    """Get help text for a surf operation."""
    result = subprocess.run(
        ["surf", op, "--help"], capture_output=True, text=True,
    )
    if result.returncode != 0:
        print(f"WARNING: surf {op} --help failed: {result.stderr}", file=sys.stderr)
        return ""
    return result.stdout


def main():
    parser = argparse.ArgumentParser(description="Generate @surf/sdk typed API client")
    parser.add_argument("--ops", nargs="*", help="Specific operations (default: all)")
    parser.add_argument("--out", default=str(Path(__file__).parent.parent / "src"),
                        help="Output directory (default: src/)")
    args = parser.parse_args()

    out = Path(args.out)

    # Get operations
    if args.ops:
        ops = args.ops
    else:
        print("Fetching all operations from surf CLI...")
        ops = get_all_operations()

    print(f"Generating SDK for {len(ops)} operations...")

    # Parse all endpoints
    endpoints: list[Endpoint] = []
    for op in ops:
        help_text = get_help(op)
        if not help_text:
            continue
        try:
            ep = parse_help(op, help_text)
            endpoints.append(ep)
        except Exception as e:
            print(f"WARNING: Failed to parse {op}: {e}", file=sys.stderr)

    print(f"Parsed {len(endpoints)} endpoints")

    # Generate
    print("\nGenerating types...")
    generate_types(endpoints, out / "data" / "types.ts")

    print("\nGenerating server categories...")
    generate_server_categories(endpoints, out / "data" / "categories")

    print("\nGenerating React hooks...")
    generate_react_hooks(endpoints, out / "react" / "hooks")

    print(f"\nDone! Generated SDK for {len(endpoints)} endpoints.")


if __name__ == "__main__":
    main()
