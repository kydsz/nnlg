import { getFileNameFromUrl } from './evalDetail'

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

/**
 * 下载附件/图片：
 * - 优先 fetch 成 blob，携带同源 Cookie 会话，并可用原文件名保存；
 * - 跨域、fetch 不可用或响应异常时回退到浏览器原生链接（后端附件接口本身也带
 *   Content-Disposition: attachment）。
 */
export async function downloadAttachment(url: string, filename?: string): Promise<void> {
  const name = filename || getFileNameFromUrl(url)
  try {
    const resp = await fetch(url, { credentials: 'include' })
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
    const blob = await resp.blob()
    if (!blob.size) throw new Error('文件为空')
    downloadBlob(blob, name)
  } catch {
    const a = document.createElement('a')
    a.href = url
    a.download = name
    a.target = '_blank'
    a.rel = 'noreferrer'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  }
}
