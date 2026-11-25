/**
 * 上传前图片压缩。
 * 仅处理 JPEG / WebP：长边超过 maxEdge 时等比缩放，并按 quality 重新编码；
 * 小于 minSizeMB、解码失败（如 HEIC）或压缩后反而变大时原样返回，保证上传不被阻断。
 */

export interface CompressImageOptions {
  /** 长边上限（像素），默认 2560 */
  maxEdge?: number
  /** 输出 JPEG 质量 0-1，默认 0.85 */
  quality?: number
  /** 小于该体积不压缩（MB），默认 2 */
  minSizeMB?: number
}

const COMPRESSIBLE = ['image/jpeg', 'image/webp']

export async function compressImage(file: File, opts: CompressImageOptions = {}): Promise<File> {
  const { maxEdge = 2560, quality = 0.85, minSizeMB = 2 } = opts
  if (!COMPRESSIBLE.includes(file.type)) return file
  if (file.size <= minSizeMB * 1024 * 1024) return file

  try {
    // imageOrientation: 'from-image' 按 EXIF 摆正，避免手机竖拍照片被旋转
    const bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
    const longest = Math.max(bitmap.width, bitmap.height)
    const scale = Math.min(1, maxEdge / longest)
    const width = Math.max(1, Math.round(bitmap.width * scale))
    const height = Math.max(1, Math.round(bitmap.height * scale))

    const canvas = document.createElement('canvas')
    canvas.width = width
    canvas.height = height
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      bitmap.close()
      return file
    }
    ctx.drawImage(bitmap, 0, 0, width, height)
    bitmap.close()

    const blob = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, 'image/jpeg', quality)
    )
    if (!blob || blob.size >= file.size) return file

    const name = file.name.replace(/\.[^.]+$/, '') + '.jpg'
    return new File([blob], name, { type: 'image/jpeg', lastModified: Date.now() })
  } catch {
    // 浏览器无法解码的格式：回退原文件，由后端大小校验兜底
    return file
  }
}
