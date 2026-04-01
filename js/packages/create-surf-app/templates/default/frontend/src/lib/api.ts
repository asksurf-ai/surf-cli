export function api(path: string) {
  return `${import.meta.env.BASE_URL}api/${path.replace(/^\/+/, '')}`
}
