import { useEffect, useRef, useState } from 'react'
import katex from 'katex'
import { Button, Popup, NavBar } from 'antd-mobile'
import 'katex/dist/katex.min.css'

interface Props {
  visible: boolean
  initialValue?: string
  onClose: () => void
  /** 确认后回调，携带 latex 字符串 */
  onConfirm: (latex: string) => void
}

const SYMBOLS = [
  '\\frac{a}{b}', '\\sqrt{x}', '\\sqrt[n]{x}', 'x^{2}', 'x_{i}',
  '\\sum_{i=1}^{n}', '\\int_{a}^{b}', '\\lim_{x \\to \\infty}',
  '\\alpha', '\\beta', '\\gamma', '\\theta', '\\pi', '\\mu', '\\sigma', '\\omega',
  '\\times', '\\div', '\\pm', '\\leq', '\\geq', '\\neq', '\\approx', '\\infty',
  '\\rightarrow', '\\Rightarrow', '\\vec{v}', '\\overline{x}', '\\underline{x}',
]

const TEMPLATES: { name: string; latex: string }[] = [
  { name: '二次方程求根', latex: 'x = \\frac{-b \\pm \\sqrt{b^2 - 4ac}}{2a}' },
  { name: '勾股定理', latex: 'a^2 + b^2 = c^2' },
  { name: '正弦定理', latex: '\\frac{a}{\\sin A} = \\frac{b}{\\sin B} = \\frac{c}{\\sin C}' },
  { name: '余弦定理', latex: 'c^2 = a^2 + b^2 - 2ab\\cos C' },
  { name: '圆面积', latex: 'S = \\pi r^2' },
  { name: '球体积', latex: 'V = \\frac{4}{3}\\pi r^3' },
  { name: '导数定义', latex: "f'(x) = \\lim_{\\Delta x \\to 0} \\frac{f(x + \\Delta x) - f(x)}{\\Delta x}" },
  { name: '定积分', latex: '\\int_{a}^{b} f(x)\\,dx' },
  { name: '牛顿第二定律', latex: 'F = ma' },
  { name: '动能定理', latex: 'E_k = \\frac{1}{2}mv^2' },
  { name: '万有引力', latex: 'F = G\\frac{m_1 m_2}{r^2}' },
  { name: '欧姆定律', latex: 'I = \\frac{U}{R}' },
  { name: '功率', latex: 'P = UI' },
  { name: '波速公式', latex: 'v = \\lambda f' },
  { name: '等差数列求和', latex: 'S_n = \\frac{n(a_1 + a_n)}{2}' },
]

/** KaTeX 公式编辑器（移动端评教 rich_text 维度用） */
export default function FormulaEditor({ visible, initialValue = '', onClose, onConfirm }: Props) {
  const [latex, setLatex] = useState(initialValue)
  const [preview, setPreview] = useState('')
  const previewRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (visible) setLatex(initialValue)
  }, [visible, initialValue])

  useEffect(() => {
    try {
      const html = katex.renderToString(latex || '', { throwOnError: false, displayMode: true })
      setPreview(html)
    } catch {
      setPreview('')
    }
  }, [latex])

  useEffect(() => {
    if (previewRef.current) previewRef.current.innerHTML = preview
  }, [preview])

  const insert = (s: string) => setLatex((v) => v + s)

  return (
    <Popup visible={visible} onMaskClick={onClose} bodyStyle={{ borderTopLeftRadius: 8, borderTopRightRadius: 8, height: '85vh', overflowY: 'auto' }}>
      <NavBar
        onBack={onClose}
        right={
          <Button
            size="small"
            color="primary"
            onClick={() => {
              onConfirm(latex)
              onClose()
            }}
          >
            确认
          </Button>
        }
      >
        插入公式
      </NavBar>
      <div style={{ padding: 12 }}>
        <div
          ref={previewRef}
          style={{
            minHeight: 60,
            border: '1px solid #eee',
            borderRadius: 6,
            padding: 8,
            marginBottom: 12,
            background: '#fafafa',
            overflowX: 'auto',
          }}
        />
        <textarea
          value={latex}
          onChange={(e) => setLatex(e.target.value)}
          placeholder="输入 LaTeX 公式，或点击下方符号/模板"
          style={{ width: '100%', minHeight: 70, border: '1px solid #ddd', borderRadius: 6, padding: 8, fontSize: 13 }}
        />
        <div style={{ margin: '12px 0 4px', fontWeight: 600 }}>符号</div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {SYMBOLS.map((s) => (
            <Button key={s} size="mini" onClick={() => insert(s)} style={{ fontSize: 11 }}>
              {s.replace(/\\/g, '').replace(/[{}]/g, '').slice(0, 8)}
            </Button>
          ))}
        </div>
        <div style={{ margin: '12px 0 4px', fontWeight: 600 }}>模板</div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 12 }}>
          {TEMPLATES.map((t) => (
            <Button key={t.name} size="mini" onClick={() => insert(t.latex)}>
              {t.name}
            </Button>
          ))}
        </div>
        <Button block fill="outline" color="danger" onClick={() => setLatex('')}>
          清空
        </Button>
      </div>
    </Popup>
  )
}
