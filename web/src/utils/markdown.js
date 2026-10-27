// 轻量 Markdown 渲染：先整体转义 HTML，再转换少量标记，避免 XSS。
const ESCAPE_MAP = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }

/** 转义 HTML 特殊字符 */
function escapeHtml(text) {
  return String(text).replace(/[&<>"']/g, (char) => ESCAPE_MAP[char])
}

/** 行内标记：`code`、**bold** */
function inline(text) {
  return text.replace(/`([^`]+)`/g, '<code>$1</code>').replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
}

/** 渲染代码块（围栏 ``` 或 ~~~） */
function fenceBlock(lines) {
  return `<pre class="waf-md-pre"><code>${lines.join('\n')}</code></pre>`
}

/**
 * 把模型返回的 Markdown 渲染为安全 HTML。
 * 支持代码块（```lang）、标题（# ~ ######）、无序/有序列表、引用、行内代码与加粗，
 * 其余按段落输出。
 */
export function renderMarkdown(source) {
  if (!source) return ''

  const lines = String(source).replace(/\r\n/g, '\n').split('\n')
  const html = []
  let listTag = ''
  // 当前所处的代码块（null 表示不在代码块内）
  let fence = null

  const closeList = () => {
    if (listTag) {
      html.push(`</${listTag}>`)
      listTag = ''
    }
  }

  for (const rawLine of lines) {
    const line = escapeHtml(rawLine).replace(/\s+$/, '')

    // 代码围栏：``` 或 ```sql（结束围栏可省略语言）
    const fenceMatch = line.match(/^\s*(?:```|~~~)\s*([A-Za-z0-9_+#-]*)\s*$/)
    if (fenceMatch) {
      if (fence) {
        html.push(fenceBlock(fence.lines))
        fence = null
      } else {
        closeList()
        fence = { lang: fenceMatch[1], lines: [] }
      }
      continue
    }
    if (fence) {
      fence.lines.push(line)
      continue
    }

    if (!line.trim()) {
      closeList()
      continue
    }

    let matched = line.match(/^(#{1,6})\s+(.*)$/)
    if (matched) {
      closeList()
      const level = Math.min(matched[1].length + 2, 6) // # → h3（配合卡片标题层级）
      html.push(`<h${level}>${inline(matched[2])}</h${level}>`)
      continue
    }

    matched = line.match(/^\s*[-*+]\s+(.*)$/)
    if (matched) {
      if (listTag !== 'ul') {
        closeList()
        html.push('<ul>')
        listTag = 'ul'
      }
      html.push(`<li>${inline(matched[1])}</li>`)
      continue
    }

    matched = line.match(/^\s*\d+[.)]\s+(.*)$/)
    if (matched) {
      if (listTag !== 'ol') {
        closeList()
        html.push('<ol>')
        listTag = 'ol'
      }
      html.push(`<li>${inline(matched[1])}</li>`)
      continue
    }

    matched = line.match(/^&gt;\s?(.*)$/)
    if (matched) {
      closeList()
      html.push(`<blockquote>${inline(matched[1])}</blockquote>`)
      continue
    }

    // 分隔线
    if (/^\s*([-*_])\1{2,}\s*$/.test(line)) {
      closeList()
      html.push('<hr />')
      continue
    }

    closeList()
    html.push(`<p>${inline(line)}</p>`)
  }

  // 未闭合的代码块也要输出，避免内容丢失
  if (fence) {
    html.push(fenceBlock(fence.lines))
  }
  closeList()
  return html.join('')
}
