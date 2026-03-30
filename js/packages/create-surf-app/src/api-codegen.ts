type JsonObject = Record<string, any>
type Endpoint = [method: string, routePath: string, spec: JsonObject]

const INTERNAL_SWAGGER_BASE =
  process.env.HERMOD_INTERNAL_URL ?? 'http://hermod-api.app.svc.cluster.local:8080'

const BLOCKED_PATHS = new Set(['/gateway/v1/onchain/sql'])

const TAG_ORDER = [
  'Market',
  'Project',
  'Wallet',
  'Token',
  'Social',
  'News',
  'Onchain',
  'Web',
  'Fund',
  'Search',
  'Prediction Market',
  'Exchange',
]

const CATEGORY_SET = new Set(TAG_ORDER)

const TYPE_MAP: Record<string, string> = {
  string: 'string',
  integer: 'number',
  number: 'number',
  boolean: 'boolean',
  object: 'any',
}

export const FALLBACK_API_TS = `/**
 * Crypto API client helpers (fallback — swagger fetch failed).
 */

export { API_BASE, normalizeProxyPath, fetchWithRetry, proxyGet, proxyPost } from './fetch'

export interface ApiResponse<T> {
  data: T[]
  meta?: { total?: number; limit?: number; offset?: number }
  error?: { code: string; message: string }
}

export interface ApiObjectResponse<T> {
  data: T
  meta?: { total?: number; limit?: number; offset?: number }
  error?: { code: string; message: string }
}

export interface CursorMeta {
  has_more: boolean
  next_cursor?: string
  limit?: number
  cached?: boolean
  credits_used?: number
}

export interface ApiCursorResponse<T> {
  data: T[]
  meta: CursorMeta
  error?: { code: string; message: string }
}
`

type GenerateApiFilesOptions = {
  swaggerUrl?: string
  logger?: (line: string) => void
}

type ParamField = {
  name: string
  type: string
  required: boolean
  in: string
  description?: string
  default?: unknown
  minimum?: number
  maximum?: number
}

function getSwaggerUrl(override?: string) {
  if (override) return override
  if (process.env.HERMOD_SWAGGER_URL) return process.env.HERMOD_SWAGGER_URL
  return `${INTERNAL_SWAGGER_BASE.replace(/\/$/, '')}/gateway/openapi.json`
}

async function fetchSwaggerDocument(swaggerUrl?: string) {
  const url = getSwaggerUrl(swaggerUrl)
  const response = await fetch(url, {
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    throw new Error(`Failed to fetch swagger from ${url}: ${response.status} ${response.statusText}`)
  }
  return await response.json() as JsonObject
}

function resolveRef(ref: string) {
  return ref.split('/').pop() ?? ref
}

function tsInterfaceName(defName: string) {
  const name = defName.split('.').pop() ?? defName
  return name ? name[0].toUpperCase() + name.slice(1) : name
}

function tagSlug(tag: string) {
  return tag.toLowerCase().replaceAll(' ', '-')
}

function isSchemaType(schema: JsonObject, checkType: string) {
  const type = schema?.type
  if (Array.isArray(type)) return type.includes(checkType)
  return type === checkType
}

function extractAllRefs(schema: JsonObject | undefined): string[] {
  if (!schema || typeof schema !== 'object') return []
  if (typeof schema.$ref === 'string') return [schema.$ref]

  const refs: string[] = []
  if (schema.items) refs.push(...extractAllRefs(schema.items))
  for (const key of ['allOf', 'anyOf', 'oneOf']) {
    for (const sub of schema[key] ?? []) refs.push(...extractAllRefs(sub))
  }
  if (schema.additionalProperties && typeof schema.additionalProperties === 'object') {
    refs.push(...extractAllRefs(schema.additionalProperties))
  }
  for (const propSchema of Object.values(schema.properties ?? {})) {
    refs.push(...extractAllRefs(propSchema as JsonObject))
  }
  return refs
}

function needsQuoting(name: string) {
  return Boolean(name) && (name.includes('-') || name.includes(' ') || /^\d/.test(name))
}

function openApiPropToTs(prop: JsonObject, definitions: JsonObject): string {
  if (prop.$ref) return tsInterfaceName(resolveRef(prop.$ref))

  if (Array.isArray(prop.allOf) && prop.allOf.length > 0) {
    return prop.allOf.map((sub: JsonObject) => openApiPropToTs(sub, definitions)).join(' & ')
  }

  for (const key of ['anyOf', 'oneOf']) {
    if (Array.isArray(prop[key]) && prop[key].length > 0) {
      return prop[key].map((sub: JsonObject) => openApiPropToTs(sub, definitions)).join(' | ')
    }
  }

  if (Array.isArray(prop.enum) && prop.enum.length > 0) {
    return prop.enum
      .map((value: unknown) => (typeof value === 'string' ? `'${value}'` : String(value)))
      .join(' | ')
  }

  let type = prop.type ?? 'unknown'
  let nullable = Boolean(prop.nullable)
  if (Array.isArray(type)) {
    nullable = nullable || type.includes('null')
    type = type.find((value: string) => value !== 'null') ?? 'unknown'
  }

  if (type === 'array') {
    const itemType = openApiPropToTs(prop.items ?? {}, definitions)
    const base = `${itemType}[]`
    return nullable ? `${base} | null` : base
  }

  if (type === 'object') {
    let base = 'Record<string, unknown>'
    if (prop.additionalProperties && typeof prop.additionalProperties === 'object') {
      base = `Record<string, ${openApiPropToTs(prop.additionalProperties, definitions)}>`
    } else if (prop.properties && typeof prop.properties === 'object') {
      const required = new Set<string>(prop.required ?? [])
      const fields = Object.entries(prop.properties).map(([name, schema]) => {
        const key = needsQuoting(name) ? `'${name}'` : name
        const optional = required.has(name) ? '' : '?'
        return `${key}${optional}: ${openApiPropToTs(schema as JsonObject, definitions)}`
      })
      base = `{ ${fields.join('; ')} }`
    }
    return nullable ? `${base} | null` : base
  }

  const tsType = TYPE_MAP[String(type)] ?? 'unknown'
  return nullable ? `${tsType} | null` : tsType
}

function schemaToTsInterface(name: string, schema: JsonObject, definitions: JsonObject) {
  const tsName = tsInterfaceName(name)

  if (Array.isArray(schema.enum) && schema.enum.length > 0) {
    const values = schema.enum
      .map((value: unknown) => (typeof value === 'string' ? `'${value}'` : String(value)))
      .join(' | ')
    return `export type ${tsName} = ${values}\n`
  }

  const properties: JsonObject = { ...(schema.properties ?? {}) }
  const extendsParts: string[] = []
  const required = new Set<string>(schema.required ?? [])

  for (const sub of schema.allOf ?? []) {
    if (sub.$ref) {
      extendsParts.push(tsInterfaceName(resolveRef(sub.$ref)))
      continue
    }
    Object.assign(properties, sub.properties ?? {})
    for (const item of sub.required ?? []) required.add(item)
  }

  if (Object.keys(properties).length === 0) {
    let nullable = Boolean(schema.nullable)
    let schemaType = schema.type
    if (Array.isArray(schemaType)) {
      nullable = nullable || schemaType.includes('null')
      schemaType = schemaType.find((value: string) => value !== 'null')
    }
    const nullSuffix = nullable ? ' | null' : ''

    if (extendsParts.length > 0) {
      return `export type ${tsName} = ${extendsParts.join(' & ')}${nullSuffix}\n`
    }
    if (isSchemaType(schema, 'array')) {
      const itemType = openApiPropToTs(schema.items ?? {}, definitions)
      return `export type ${tsName} = ${itemType}[]${nullSuffix}\n`
    }
    if (schema.additionalProperties && typeof schema.additionalProperties === 'object') {
      const valueType = openApiPropToTs(schema.additionalProperties, definitions)
      return `export type ${tsName} = Record<string, ${valueType}>${nullSuffix}\n`
    }
    if (typeof schemaType === 'string' && TYPE_MAP[schemaType] && schemaType !== 'object') {
      return `export type ${tsName} = ${TYPE_MAP[schemaType]}${nullSuffix}\n`
    }
    return `export type ${tsName} = Record<string, unknown>${nullSuffix}\n`
  }

  const extendsClause = extendsParts.length > 0 ? ` extends ${extendsParts.join(', ')}` : ''
  const lines = [`export interface ${tsName}${extendsClause} {`]
  for (const [propName, propSchema] of Object.entries(properties)) {
    if (propName === '$schema') continue
    const description = (propSchema as JsonObject).description
    if (description) lines.push(`  /** ${String(description).replaceAll('\n', ' ').trim()} */`)
    const key = needsQuoting(propName) ? `'${propName}'` : propName
    const optional = required.has(propName) ? '' : '?'
    lines.push(`  ${key}${optional}: ${openApiPropToTs(propSchema as JsonObject, definitions)}`)
  }
  lines.push('}')
  return `${lines.join('\n')}\n`
}

function isDataTypeRef(refName: string) {
  if (
    refName.startsWith('DataResponse') ||
    refName.startsWith('DataObjectResponse') ||
    refName.startsWith('SimpleListResponse') ||
    refName.startsWith('CursorDataResponse') ||
    refName.endsWith('InputBody') ||
    ['DataAPIError', 'ResponseMeta', 'ObjectResponseMeta', 'OffsetMeta', 'CursorMeta'].includes(refName)
  ) {
    return false
  }
  return true
}

function isTypedResponse(refName: string) {
  return !refName.includes('RawJSON') && !refName.includes('MapStringInterface')
}

function getResponseModel(pathSpec: JsonObject): [string | null, boolean] {
  const schema =
    pathSpec.responses?.['200']?.content?.['application/json']?.schema ??
    pathSpec.responses?.['200']?.schema ??
    {}
  if (schema.$ref) {
    const refName = resolveRef(schema.$ref)
    return [refName, isTypedResponse(refName)]
  }
  return [null, false]
}

function isObjectResponse(respModel: string | null) {
  return Boolean(respModel && respModel.startsWith('DataObjectResponse'))
}

function isCursorResponse(respModel: string | null) {
  return Boolean(respModel && respModel.startsWith('CursorDataResponse'))
}

function isArrayResponse(respModel: string) {
  return (
    respModel.startsWith('DataResponse') ||
    respModel.startsWith('SimpleListResponse') ||
    respModel.startsWith('CursorDataResponse')
  )
}

function extractDataItemDefName(respModel: string | null, definitions: JsonObject) {
  if (!respModel) return null
  const objectResponse = respModel.startsWith('DataObjectResponse')
  const arrayResponse = isArrayResponse(respModel) && !objectResponse
  if (!objectResponse && !arrayResponse) return null
  const modelDef = definitions[respModel] ?? {}
  const dataProp = modelDef.properties?.data ?? {}

  if (objectResponse) {
    if (dataProp.$ref) {
      const refName = resolveRef(dataProp.$ref)
      return isTypedResponse(refName) ? refName : null
    }
    return null
  }

  const itemRef = dataProp.items?.$ref
  if (!itemRef) return null
  const refName = resolveRef(itemRef)
  return isTypedResponse(refName) ? refName : null
}

function extractDataItemType(respModel: string | null, definitions: JsonObject) {
  const defName = extractDataItemDefName(respModel, definitions)
  return defName ? tsInterfaceName(defName) : null
}

function getBodyParam(params: JsonObject[], spec?: JsonObject) {
  for (const param of params) {
    if (param.in === 'body') return param
  }
  const schema = spec?.requestBody?.content?.['application/json']?.schema
  if (schema) return { in: 'body', schema }
  return null
}

function buildParamsInterface(params: JsonObject[]) {
  const result: ParamField[] = []
  for (const param of params) {
    if (param.in === 'body') continue
    const schema = param.schema ?? {}
    let paramType = param.type ?? schema.type ?? 'string'
    const enumValues = schema.enum
    let tsType = 'string'
    if (Array.isArray(enumValues) && enumValues.length > 0) {
      tsType = enumValues
        .map((value: unknown) => (typeof value === 'string' ? `'${value}'` : String(value)))
        .join(' | ')
    } else {
      if (Array.isArray(paramType)) {
        paramType = paramType.find((value: string) => value !== 'null') ?? 'string'
      }
      tsType = TYPE_MAP[String(paramType)] ?? 'string'
    }
    result.push({
      name: param.name,
      type: tsType,
      required: Boolean(param.required),
      in: param.in ?? 'query',
      description: schema.description ?? param.description,
      default: schema.default ?? param.default,
      minimum: schema.minimum,
      maximum: schema.maximum,
    })
  }
  return result
}

function collectRefsFromSchema(schema: JsonObject, refs: Set<string>) {
  for (const ref of extractAllRefs(schema)) refs.add(tsInterfaceName(resolveRef(ref)))
}

function collectBodyTypeRefs(spec: JsonObject) {
  const refs = new Set<string>()
  const bodyParam = getBodyParam(spec.parameters ?? [], spec)
  if (!bodyParam) return refs
  const bodySchema = bodyParam.schema ?? {}
  const definition = bodySchema.$ref ? bodySchema : bodySchema
  collectRefsFromSchema(definition, refs)
  return refs
}

function collectSubRefs(names: Set<string>, definitions: JsonObject) {
  const queue = [...names]
  while (queue.length > 0) {
    const defName = queue.pop()!
    const schema = definitions[defName] ?? {}
    for (const refString of extractAllRefs(schema)) {
      const refName = resolveRef(refString)
      if (isDataTypeRef(refName) && !names.has(refName) && definitions[refName]) {
        names.add(refName)
        queue.push(refName)
      }
    }
  }
}

function categorizeSchemas(definitions: JsonObject, paths: JsonObject) {
  const tagSchemas = new Map<string, Set<string>>()

  for (const routePath of Object.keys(paths).sort()) {
    const methods = paths[routePath] ?? {}
    for (const [method, spec] of Object.entries(methods)) {
      if (!['get', 'post', 'put', 'delete'].includes(method)) continue
      const tags = (spec as JsonObject).tags ?? []
      if (!tags[0] || !CATEGORY_SET.has(tags[0])) continue
      const tag = tags[0]

      const [respModel] = getResponseModel(spec as JsonObject)
      const itemDef = extractDataItemDefName(respModel, definitions)
      if (itemDef && isDataTypeRef(itemDef)) {
        if (!tagSchemas.has(tag)) tagSchemas.set(tag, new Set<string>())
        tagSchemas.get(tag)!.add(itemDef)
      }

      const bodyParam = getBodyParam((spec as JsonObject).parameters ?? [], spec as JsonObject)
      if (bodyParam) {
        const bodySchema = bodyParam.schema ?? {}
        const bodyDef = bodySchema.$ref ? definitions[resolveRef(bodySchema.$ref)] ?? {} : bodySchema
        for (const refString of extractAllRefs(bodyDef)) {
          const refName = resolveRef(refString)
          if (!isDataTypeRef(refName)) continue
          if (!tagSchemas.has(tag)) tagSchemas.set(tag, new Set<string>())
          tagSchemas.get(tag)!.add(refName)
        }
      }
    }
  }

  const result: Record<string, string[]> = {}
  for (const [tag, names] of tagSchemas.entries()) {
    collectSubRefs(names, definitions)
    result[tag] = [...names].sort()
  }
  return result
}

function topoSortSchemas(names: string[], definitions: JsonObject) {
  const nameSet = new Set(names)
  const deps = new Map<string, Set<string>>()
  for (const name of names) {
    const schema = definitions[name] ?? {}
    const refs = new Set<string>()
    for (const propSchema of Object.values(schema.properties ?? {})) {
      for (const refString of extractAllRefs(propSchema as JsonObject)) {
        const refName = resolveRef(refString)
        if (nameSet.has(refName) && refName !== name) refs.add(refName)
      }
    }
    deps.set(name, refs)
  }

  const dependents = new Map<string, Set<string>>()
  const inDegree = new Map<string, number>()
  for (const name of names) {
    dependents.set(name, new Set())
    inDegree.set(name, 0)
  }
  for (const [name, nameDeps] of deps.entries()) {
    for (const dep of nameDeps) {
      dependents.get(dep)?.add(name)
      inDegree.set(name, (inDegree.get(name) ?? 0) + 1)
    }
  }

  const queue = names.filter(name => (inDegree.get(name) ?? 0) === 0).sort()
  const result: string[] = []
  while (queue.length > 0) {
    const name = queue.shift()!
    result.push(name)
    for (const dependent of [...(dependents.get(name) ?? [])].sort()) {
      const next = (inDegree.get(dependent) ?? 0) - 1
      inDegree.set(dependent, next)
      if (next === 0) queue.push(dependent)
    }
  }

  for (const name of [...names].sort()) {
    if (!result.includes(name)) result.push(name)
  }
  return result
}

function buildTypesCommon() {
  return `/**
 * Common types — auto-generated from hermod OpenAPI spec.
 */

export interface ResponseMeta {
  total?: number
  limit?: number
  offset?: number
  cached?: boolean
  credits_used?: number
}

export interface ApiResponse<T> {
  data: T[]
  meta?: ResponseMeta
  error?: { code: string; message: string }
}

export interface ApiObjectResponse<T> {
  data: T
  meta?: ResponseMeta
  error?: { code: string; message: string }
}

export interface CursorMeta {
  has_more: boolean
  next_cursor?: string
  limit?: number
  cached?: boolean
  credits_used?: number
}

export interface ApiCursorResponse<T> {
  data: T[]
  meta: CursorMeta
  error?: { code: string; message: string }
}
`
}

function buildTypesCategory(tag: string, schemaNames: string[], definitions: JsonObject) {
  const lines = ['/**', ` * ${tag} types — auto-generated from hermod OpenAPI spec.`, ' */', '']
  for (const name of schemaNames) lines.push(schemaToTsInterface(name, definitions[name] ?? {}, definitions))
  return lines.join('\n')
}

function funcNameFromPath(routePath: string) {
  const clean = routePath.replace('/gateway/v1/', '').replace('/v1/', '')
  const parts: string[] = []
  for (const segment of clean.split('/')) {
    if (segment.startsWith('{') && segment.endsWith('}')) continue
    parts.push(segment)
  }
  const name = parts
    .flatMap(part => part.split('-'))
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join('')
  return `fetch${name}`
}

function hookNameFromFunc(funcName: string) {
  return `use${funcName.slice(5)}`
}

function swaggerTypeToTs(prop: JsonObject): string {
  if (prop.$ref) return tsInterfaceName(resolveRef(prop.$ref))
  if (isSchemaType(prop, 'array')) return `${swaggerTypeToTs(prop.items ?? {})}[]`
  let type = prop.type ?? 'any'
  if (Array.isArray(type)) type = type.find((value: string) => value !== 'null') ?? 'any'
  return TYPE_MAP[String(type)] ?? 'any'
}

function buildApiCore(tagsWithContent: string[], typesTags: string[], forBackend = false) {
  const generatedAt = new Date().toISOString()
  const lines = [
    '/**',
    ' * Crypto API client — auto-generated from hermod OpenAPI spec.',
    ` * Generated at: ${generatedAt}`,
    ' *',
    ...(forBackend
      ? [
          ' * Backend-only: CommonJS fetch functions, no React Query hooks.',
          " * Usage: const { fetchMarketPrice } = require('./api')",
        ]
      : [
          ' * Core helpers + barrel re-exports from per-category modules.',
          " * Usage: import { useMarketPrice, proxyGet } from '@/lib/api'",
        ]),
    ' */',
    '',
  ]

  if (forBackend) {
    lines.push(
      "const DATA_PROXY_BASE = process.env.DATA_PROXY_BASE || 'http://127.0.0.1:9999/proxy'",
      '',
      'function normalizeProxyPath(path) {',
      "  const trimmed = String(path || '').replace(/^\\/+/, '')",
      "  return trimmed.replace(/^(?:proxy\\/)+/, '')",
      '}',
      '',
      'async function fetchWithRetry(url, init, retries = 1) {',
      '  for (let attempt = 0; attempt <= retries; attempt++) {',
      '    const res = await fetch(url, init)',
      '    const text = await res.text()',
      '    if (text) return JSON.parse(text)',
      '    if (attempt < retries) await new Promise(resolve => setTimeout(resolve, 1000))',
      '  }',
      "  throw new Error(`Empty response from ${url.replace(DATA_PROXY_BASE, '')}`)",
      '}',
      '',
      'async function proxyGet(path, params) {',
      "  const qs = params ? '?' + new URLSearchParams(params).toString() : ''",
      '  return fetchWithRetry(`${DATA_PROXY_BASE}/${normalizeProxyPath(path)}${qs}`)',
      '}',
      '',
      'async function proxyPost(path, body) {',
      '  return fetchWithRetry(`${DATA_PROXY_BASE}/${normalizeProxyPath(path)}`, {',
      "    method: 'POST',",
      "    headers: { 'Content-Type': 'application/json' },",
      '    body: body ? JSON.stringify(body) : undefined,',
      '  })',
      '}',
      '',
      'module.exports.DATA_PROXY_BASE = DATA_PROXY_BASE',
      'module.exports.normalizeProxyPath = normalizeProxyPath',
      'module.exports.fetchWithRetry = fetchWithRetry',
      'module.exports.proxyGet = proxyGet',
      'module.exports.proxyPost = proxyPost',
      '',
    )
    for (const tag of tagsWithContent) {
      lines.push(`Object.assign(module.exports, require('./api-${tagSlug(tag)}'))`)
    }
    return lines.join('\n')
  }

  lines.push("export { API_BASE, normalizeProxyPath, fetchWithRetry, proxyGet, proxyPost } from './fetch'", '')
  lines.push("export * from './types-common'")
  for (const tag of typesTags) lines.push(`export * from './types-${tagSlug(tag)}'`)
  lines.push('')
  for (const tag of tagsWithContent) lines.push(`export * from './api-${tagSlug(tag)}'`)
  return lines.join('\n')
}

function buildApiIndex(tagPaths: Record<string, Endpoint[]>, definitions: JsonObject, forBackend = false) {
  const lines = ['# API Index', '']
  const ext = forBackend ? 'js' : 'ts'
  lines.push(`Read \`api-{category}.${ext}\` for full signatures and types.`, '')

  for (const tag of TAG_ORDER) {
    const endpoints = tagPaths[tag] ?? []
    if (endpoints.length === 0) continue
    lines.push(`## ${tag} — \`api-${tagSlug(tag)}.${ext}\``)
    for (const [, routePath, spec] of endpoints) {
      const funcName = funcNameFromPath(routePath)
      const params = buildParamsInterface(spec.parameters ?? [])
      const bodyParam = getBodyParam(spec.parameters ?? [], spec)
      const required = params.filter(param => param.required).map(param => param.name)
      if (bodyParam?.schema?.$ref) {
        const bodyDef = definitions[resolveRef(bodyParam.schema.$ref)] ?? {}
        required.push(...Object.keys(bodyDef.properties ?? {}).filter(name => (bodyDef.required ?? []).includes(name)))
      }
      const signature = required.length > 0 ? `(${required.join(', ')})` : '()'
      const description = spec.description || spec.summary
      lines.push(`- \`${funcName}${signature}\`${description ? ` — ${description}` : ''}`)
    }
    lines.push('')
  }

  return lines.join('\n')
}

function buildQueryKey(funcName: string) {
  const parts = funcName.slice(5)
  const tokens: string[] = []
  let current = ''
  for (const char of parts) {
    if (/[A-Z]/.test(char) && current) {
      tokens.push(current.toLowerCase())
      current = char
      continue
    }
    current += char
  }
  if (current) tokens.push(current.toLowerCase())
  return tokens
}

function buildQueryAssignments(queryOnly: ParamField[], paramsOptional: boolean) {
  const lines: string[] = []
  for (const param of queryOnly) {
    if (param.required) {
      lines.push(`  qs['${param.name}'] = String(params.${param.name})`)
      continue
    }
    if (param.default !== undefined) {
      const defaultLiteral =
        typeof param.default === 'string' ? `'${param.default}'` : JSON.stringify(param.default)
      lines.push(`  qs['${param.name}'] = String(params?.${param.name} ?? ${defaultLiteral})`)
      continue
    }
    lines.push(`  if (params?.${param.name} !== undefined) qs['${param.name}'] = String(params.${param.name})`)
  }
  return lines
}

function buildApiCategory(
  tag: string,
  endpoints: Endpoint[],
  definitions: JsonObject,
  availableTypes: Set<string>,
  includeHooks = true,
) {
  const isBackend = !includeHooks
  const generatedAt = new Date().toISOString()
  const lines = [
    '/**',
    ` * ${tag} API — auto-generated from hermod OpenAPI spec.`,
    ` * Generated at: ${generatedAt}`,
    ' */',
    '',
  ]

  const usedTypes = new Set<string>()
  let hasApiResponse = false
  let hasApiObjectResponse = false
  let hasApiCursorResponse = false

  for (const [, , spec] of endpoints) {
    const [respModel] = getResponseModel(spec)
    const itemType = extractDataItemType(respModel, definitions)
    if (itemType && availableTypes.has(itemType)) {
      usedTypes.add(itemType)
      if (isObjectResponse(respModel)) hasApiObjectResponse = true
      else if (isCursorResponse(respModel)) hasApiCursorResponse = true
      else hasApiResponse = true
    }
    for (const ref of collectBodyTypeRefs(spec)) {
      if (availableTypes.has(ref)) usedTypes.add(ref)
    }
  }

  if (isBackend) {
    lines.push("'use strict'", '', "const { proxyGet, proxyPost } = require('./api')", '')
  } else {
    const imports = ['proxyGet', 'proxyPost']
    const typeImports: string[] = []
    if (hasApiResponse) typeImports.push('ApiResponse')
    if (hasApiObjectResponse) typeImports.push('ApiObjectResponse')
    if (hasApiCursorResponse) typeImports.push('ApiCursorResponse')
    for (const typeName of [...usedTypes].sort()) typeImports.push(typeName)
    const allImports = [
      ...imports,
      ...typeImports.map(typeName => `type ${typeName}`),
    ]
    lines.push(`import { ${allImports.join(', ')} } from './api'`)
    if (includeHooks) {
      lines.push(
        "import { useInfiniteQuery, useQuery, type UseInfiniteQueryOptions, type UseQueryOptions } from '@tanstack/react-query'",
      )
    }
    lines.push('')
  }

  const generatedFuncs: Array<{
    funcName: string
    hasParams: boolean
    paramsOptional: boolean
    returnType: string
    isQuery: boolean
    paginationType: 'offset' | 'cursor' | null
    summary: string
  }> = []

  for (const [httpMethod, routePath, spec] of endpoints) {
    const funcName = funcNameFromPath(routePath)
    const params = buildParamsInterface(spec.parameters ?? [])
    const bodyParam = getBodyParam(spec.parameters ?? [], spec)
    const pathParams = params.filter(param => param.in === 'path')
    const queryOnly = params.filter(param => param.in === 'query')
    const hasRequired = params.some(param => param.required)
    const hasBody = bodyParam !== null
    const paramsOptional = !hasRequired && !hasBody
    const [respModel, typedResponse] = getResponseModel(spec)
    const itemType = extractDataItemType(respModel, definitions)
    const isCursor = isCursorResponse(respModel)

    let returnType = 'any'
    if (typedResponse && itemType && availableTypes.has(itemType)) {
      if (isObjectResponse(respModel)) returnType = `ApiObjectResponse<${itemType}>`
      else if (isCursor) returnType = `ApiCursorResponse<${itemType}>`
      else returnType = `ApiResponse<${itemType}>`
    }

    const bodyFields: Array<[string, JsonObject]> = []
    const bodyRequired = new Set<string>()
    if (hasBody) {
      const bodySchema = bodyParam?.schema ?? {}
      const bodyDefinition = bodySchema.$ref ? definitions[resolveRef(bodySchema.$ref)] ?? {} : bodySchema
      for (const field of bodyDefinition.required ?? []) bodyRequired.add(field)
      for (const [name, schema] of Object.entries(bodyDefinition.properties ?? {})) {
        if (name === '$schema') continue
        bodyFields.push([name, schema as JsonObject])
      }
    }

    const paramFields: string[] = []
    for (const param of params) {
      const optional = param.required ? '' : '?'
      paramFields.push(`${param.name}${optional}: ${param.type}`)
    }
    for (const [name, schema] of bodyFields) {
      const optional = bodyRequired.has(name) ? '' : '?'
      paramFields.push(`${name}${optional}: ${swaggerTypeToTs(schema)}`)
    }

    lines.push(`/** ${spec.description || spec.summary || funcName} */`)
    if (isBackend) {
      if (paramFields.length > 0) lines.push(`async function ${funcName}(params) {`)
      else lines.push(`async function ${funcName}() {`)
    } else {
      if (paramFields.length > 0) {
        const paramsType = `{ ${paramFields.join('; ')} }`
        lines.push(`export async function ${funcName}(params${paramsOptional ? '?' : ''}: ${paramsType}) {`)
      } else {
        lines.push(`export async function ${funcName}() {`)
      }
    }

    for (const param of queryOnly.filter(param => param.minimum !== undefined || param.maximum !== undefined)) {
      const accessor = `params${paramsOptional ? '?' : ''}.${param.name}`
      let expr = accessor
      if (param.minimum !== undefined && param.maximum !== undefined) {
        expr = `Math.max(${param.minimum}, Math.min(${param.maximum}, ${accessor}))`
      } else if (param.minimum !== undefined) {
        expr = `Math.max(${param.minimum}, ${accessor})`
      } else if (param.maximum !== undefined) {
        expr = `Math.min(${param.maximum}, ${accessor})`
      }
      if (param.required) lines.push(`  params.${param.name} = ${expr}`)
      else lines.push(`  if (${accessor} !== undefined) params.${param.name} = ${expr}`)
    }

    let proxyPath = routePath.replace('/gateway/v1/', '').replace('/v1/', '')
    for (const param of pathParams) {
      proxyPath = proxyPath.replace(`{${param.name}}`, `\${encodeURIComponent(params.${param.name})}`)
    }

    if (httpMethod === 'POST' || httpMethod === 'PUT') {
      let bodyExpr = 'params'
      if (queryOnly.length > 0 && bodyFields.length > 0) {
        lines.push(`  const { ${queryOnly.map(param => param.name).join(', ')}, ...body } = params`)
        bodyExpr = 'body'
      }
      if (queryOnly.length > 0) {
        lines.push(isBackend ? '  const qs = {}' : '  const qs: Record<string, string> = {}')
        lines.push(...buildQueryAssignments(queryOnly, paramsOptional))
        lines.push(
          isBackend
            ? `  return proxyPost(\`${proxyPath}?\${new URLSearchParams(qs)}\`${bodyFields.length > 0 ? `, ${bodyExpr}` : ''})`
            : `  return proxyPost<${returnType}>(\`${proxyPath}?\${new URLSearchParams(qs)}\`${bodyFields.length > 0 ? `, ${bodyExpr}` : ''})`,
        )
      } else if (bodyFields.length > 0) {
        lines.push(isBackend ? `  return proxyPost(\`${proxyPath}\`, params)` : `  return proxyPost<${returnType}>(\`${proxyPath}\`, params)`)
      } else {
        lines.push(isBackend ? `  return proxyPost(\`${proxyPath}\`)` : `  return proxyPost<${returnType}>(\`${proxyPath}\`)`)
      }
    } else {
      if (queryOnly.length > 0) {
        lines.push(isBackend ? '  const qs = {}' : '  const qs: Record<string, string> = {}')
        lines.push(...buildQueryAssignments(queryOnly, paramsOptional))
        lines.push(
          isBackend
            ? `  return proxyGet(\`${proxyPath}\`, qs)`
            : `  return proxyGet<${returnType}>(\`${proxyPath}\`, qs)`,
        )
      } else {
        lines.push(isBackend ? `  return proxyGet(\`${proxyPath}\`)` : `  return proxyGet<${returnType}>(\`${proxyPath}\`)`)
      }
    }

    lines.push('}', '')

    const isMutation = (httpMethod === 'POST' || httpMethod === 'PUT') && (tag === 'Onchain' || tag === 'Web')
    const queryParamNames = new Set(queryOnly.map(param => param.name))
    let paginationType: 'offset' | 'cursor' | null = null
    if (!isMutation && respModel && isArrayResponse(respModel)) {
      if (isCursor && queryParamNames.has('cursor')) paginationType = 'cursor'
      else if (queryParamNames.has('limit') && queryParamNames.has('offset')) paginationType = 'offset'
    }

    generatedFuncs.push({
      funcName,
      hasParams: paramFields.length > 0,
      paramsOptional,
      returnType,
      isQuery: !isMutation,
      paginationType,
      summary: spec.summary ?? '',
    })
  }

  if (isBackend) {
    lines.push('module.exports = {')
    for (const generated of generatedFuncs) lines.push(`  ${generated.funcName},`)
    lines.push('}', '')
    return lines.join('\n')
  }

  const queryFuncs = generatedFuncs.filter(generated => generated.isQuery)
  if (includeHooks && queryFuncs.length > 0) {
    lines.push('// Hooks', '')
    lines.push("type QueryOpts<T> = Omit<UseQueryOptions<T, Error>, 'queryKey' | 'queryFn'>", '')
    for (const generated of queryFuncs) {
      const hookName = hookNameFromFunc(generated.funcName)
      const keyTokens = buildQueryKey(generated.funcName).map(token => `'${token}'`).join(', ')
      if (generated.hasParams) {
        lines.push(`export function ${hookName}(params${generated.paramsOptional ? '?' : ''}: Parameters<typeof ${generated.funcName}>[0], opts?: QueryOpts<${generated.returnType}>) {`)
        lines.push(`  return useQuery({ queryKey: [${keyTokens}, params], queryFn: () => ${generated.funcName}(params${generated.paramsOptional ? '' : '!'}), ...opts })`)
      } else {
        lines.push(`export function ${hookName}(opts?: QueryOpts<${generated.returnType}>) {`)
        lines.push(`  return useQuery({ queryKey: [${keyTokens}], queryFn: () => ${generated.funcName}(), ...opts })`)
      }
      lines.push('}', '')
    }

    const paginated = queryFuncs.filter(generated => generated.paginationType !== null)
    if (paginated.length > 0) {
      lines.push('// Infinite query hooks', '')
      if (paginated.some(generated => generated.paginationType === 'offset')) {
        lines.push("type OffsetInfiniteOpts<T> = Omit<UseInfiniteQueryOptions<T, Error, T, T, unknown[], number>, 'queryKey' | 'queryFn' | 'initialPageParam' | 'getNextPageParam'>")
      }
      if (paginated.some(generated => generated.paginationType === 'cursor')) {
        lines.push("type CursorInfiniteOpts<T> = Omit<UseInfiniteQueryOptions<T, Error, T, T, unknown[], string>, 'queryKey' | 'queryFn' | 'initialPageParam' | 'getNextPageParam'>")
      }
      lines.push('')
      for (const generated of paginated) {
        const hookName = hookNameFromFunc(generated.funcName).replace(/^use/, 'useInfinite')
        const keyTokens = buildQueryKey(generated.funcName).map(token => `'${token}'`).join(', ')
        if (generated.paginationType === 'cursor') {
          lines.push(`export function ${hookName}(params${generated.paramsOptional ? '?' : ''}: Omit<Parameters<typeof ${generated.funcName}>[0], 'cursor'>, opts?: CursorInfiniteOpts<${generated.returnType}>) {`)
          lines.push('  return useInfiniteQuery({')
          lines.push(`    queryKey: [${keyTokens}, 'infinite', params],`)
          lines.push(`    queryFn: ({ pageParam }) => ${generated.funcName}({ ...params${generated.paramsOptional ? '' : '!'}, ...(pageParam ? { cursor: pageParam } : {}) }),`)
          lines.push("    initialPageParam: '',")
          lines.push("    getNextPageParam: (lastPage) => {")
          lines.push("      const meta = (lastPage as any)?.meta")
          lines.push('      if (!meta?.has_more || !meta?.next_cursor) return undefined')
          lines.push('      return meta.next_cursor')
          lines.push('    },')
          lines.push('    ...opts,')
          lines.push('  })')
          lines.push('}', '')
        } else {
          lines.push(`export function ${hookName}(params${generated.paramsOptional ? '?' : ''}: Omit<Parameters<typeof ${generated.funcName}>[0], 'offset'>, opts?: OffsetInfiniteOpts<${generated.returnType}>) {`)
          lines.push('  return useInfiniteQuery({')
          lines.push(`    queryKey: [${keyTokens}, 'infinite', params],`)
          lines.push(`    queryFn: ({ pageParam = 0 }) => ${generated.funcName}({ ...params${generated.paramsOptional ? '' : '!'}, offset: pageParam }),`)
          lines.push('    initialPageParam: 0,')
          lines.push('    getNextPageParam: (lastPage) => {')
          lines.push("      const meta = (lastPage as any)?.meta")
          lines.push('      if (!meta?.total || !meta?.limit) return undefined')
          lines.push('      const next = (meta.offset ?? 0) + meta.limit')
          lines.push('      return next < meta.total ? next : undefined')
          lines.push('    },')
          lines.push('    ...opts,')
          lines.push('  })')
          lines.push('}', '')
        }
      }
    }
  }

  return lines.join('\n')
}

export async function generateApiFiles({
  swaggerUrl,
  logger = () => {},
}: GenerateApiFilesOptions = {}) {
  const doc = await fetchSwaggerDocument(swaggerUrl)
  const definitions = doc.components?.schemas ?? {}
  const paths = doc.paths ?? {}
  const tagPaths: Record<string, Endpoint[]> = {}

  for (const routePath of Object.keys(paths).sort()) {
    const methods = paths[routePath] ?? {}
    for (const [method, spec] of Object.entries(methods)) {
      if (!['get', 'post', 'put', 'delete'].includes(method)) continue
      const tags = (spec as JsonObject).tags ?? []
      if (!tags[0] || !CATEGORY_SET.has(tags[0])) continue
      if (BLOCKED_PATHS.has(routePath)) continue
      if (!tagPaths[tags[0]]) tagPaths[tags[0]] = []
      tagPaths[tags[0]].push([method.toUpperCase(), routePath, spec as JsonObject])
    }
  }

  const categorySchemas = categorizeSchemas(definitions, paths)
  const availableTypes = new Set<string>()
  for (const names of Object.values(categorySchemas)) {
    for (const name of names) availableTypes.add(tsInterfaceName(name))
  }

  const categoryFiles: Record<string, string> = {}
  const tagsWithContent: string[] = []
  for (const tag of TAG_ORDER) {
    const endpoints = tagPaths[tag] ?? []
    if (endpoints.length === 0) continue
    tagsWithContent.push(tag)
    categoryFiles[`api-${tagSlug(tag)}.ts`] = buildApiCategory(tag, endpoints, definitions, availableTypes, true)
  }

  const typesTags: string[] = []
  const result: Record<string, string> = {
    'types-common.ts': buildTypesCommon(),
  }

  for (const tag of TAG_ORDER) {
    const schemaNames = categorySchemas[tag] ?? []
    if (schemaNames.length === 0) continue
    typesTags.push(tag)
    result[`types-${tagSlug(tag)}.ts`] = buildTypesCategory(
      tag,
      topoSortSchemas(schemaNames, definitions),
      definitions,
    )
  }

  result['api.ts'] = buildApiCore(tagsWithContent, typesTags, false)
  result['API_INDEX.md'] = buildApiIndex(tagPaths, definitions, false)
  Object.assign(result, categoryFiles)

  result['backend/api.js'] = buildApiCore(tagsWithContent, typesTags, true)
  result['backend/API_INDEX.md'] = buildApiIndex(tagPaths, definitions, true)
  for (const tag of TAG_ORDER) {
    const endpoints = tagPaths[tag] ?? []
    if (endpoints.length === 0) continue
    result[`backend/api-${tagSlug(tag)}.js`] = buildApiCategory(tag, endpoints, definitions, availableTypes, false)
  }

  logger(`  generated ${Object.keys(result).length} API files from ${getSwaggerUrl(swaggerUrl)}`)
  return result
}
