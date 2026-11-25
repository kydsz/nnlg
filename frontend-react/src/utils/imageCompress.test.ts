import { describe, it, expect } from 'vitest'
import { compressImage } from './imageCompress'

describe('compressImage', () => {
  it('非 JPEG/WebP 图片原样返回', async () => {
    const f = new File(['png'], 'a.png', { type: 'image/png' })
    await expect(compressImage(f)).resolves.toBe(f)
  })

  it('小于阈值的 JPEG 不压缩', async () => {
    const f = new File([new Uint8Array(1024)], 'small.jpg', { type: 'image/jpeg' })
    await expect(compressImage(f)).resolves.toBe(f)
  })

  it('非图片文件原样返回', async () => {
    const f = new File(['pdf'], 'a.pdf', { type: 'application/pdf' })
    await expect(compressImage(f)).resolves.toBe(f)
  })
})
