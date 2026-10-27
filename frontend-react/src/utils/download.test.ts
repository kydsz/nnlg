import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { downloadAttachment } from './download'

const createObjectURL = vi.fn(() => 'blob:mock')
const revokeObjectURL = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn())
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  createObjectURL.mockClear()
  revokeObjectURL.mockClear()
})

describe('downloadAttachment', () => {
  it('同源 fetch 成功后按原文件名下载，并去掉上传 uuid 前缀', async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      blob: async () => new Blob(['image-bytes']),
    } as Response)
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await downloadAttachment(
      '/api/v1/files/evaluations/3/class_photo/0f8fad5b-d9cb-469f-a165-70867728950e_现场照片.jpg'
    )

    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/files/evaluations/3/class_photo/0f8fad5b-d9cb-469f-a165-70867728950e_现场照片.jpg',
      { credentials: 'include' }
    )
    expect(createObjectURL).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
    expect(revokeObjectURL).toHaveBeenCalled()
  })

  it('fetch 不可用时回退到浏览器原生下载链接', async () => {
    vi.mocked(fetch).mockRejectedValue(new Error('cors'))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    await downloadAttachment('/files/photo.png')
    expect(click).toHaveBeenCalled()
  })
})
