import { request } from '../http'

export const uploadApi = {
  upload: (file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return request<{ url: string; filename: string }>({
      url: '/upload',
      method: 'POST',
      data: fd,
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  uploadEvaluationFile: (taskId: number, dimCode: string, file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return request<{ url: string; filename: string }>({
      url: `/upload/evaluation/${taskId}/${dimCode}`,
      method: 'POST',
      data: fd,
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },

  deleteEvaluationFile: (taskId: number, dimCode: string, filename: string) =>
    request<null>({
      url: `/upload/evaluation/${taskId}/${dimCode}/${filename}`,
      method: 'DELETE',
    }),

  serveUrl: (filepath: string) => `/api/v1/files/${filepath}`,
}
