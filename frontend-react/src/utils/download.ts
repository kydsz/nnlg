/** Blob 下载工具 */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

/** 从 Content-Disposition 或默认名生成文件名 */
export function exportFilename(prefix: string, ext = 'xlsx'): string {
  const ts = new Date().toISOString().slice(0, 10)
  return `${prefix}_${ts}.${ext}`
}
