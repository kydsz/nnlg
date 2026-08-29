import { useEffect, useState } from 'react'

/** UA + 宽度判断移动端 */
export function isMobileDevice(): boolean {
  if (typeof window === 'undefined') return false
  const ua = navigator.userAgent
  const uaMobile = /Android|iPhone|iPad|iPod|Mobile|HarmonyOS/i.test(ua)
  const narrow = window.matchMedia('(max-width: 768px)').matches
  return uaMobile || narrow
}

export function useIsMobile(): boolean {
  const [mobile, setMobile] = useState(isMobileDevice)
  useEffect(() => {
    const mq = window.matchMedia('(max-width: 768px)')
    const handler = () => setMobile(isMobileDevice())
    mq.addEventListener?.('change', handler)
    return () => mq.removeEventListener?.('change', handler)
  }, [])
  return mobile
}
