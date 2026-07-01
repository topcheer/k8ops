// --- Chat ---
import { escapeHtml } from './modules/utils.js';

let currentConvId = null;
let pendingToolCalls = ''; // accumulate tool calls before final answer

export function openChatOverlay() {
  document.getElementById('chatOverlay').classList.add('active');
  if (!currentConvId) {
    document.getElementById('chatMessages').innerHTML =
      '<div style="text-align:center;color:var(--text-muted);padding:80px 40px;">Start a conversation with k8ops AI</div>' +
      '<div style="text-align:center;color:var(--text-faded);font-size:12px;">Ask questions like: <code style="color:var(--accent-blue);">show pods in default namespace</code> or <code style="color:var(--accent-blue);">why is my pod crashing?</code></div>';
  }
  // Check provider status
  checkChatProvider();
  document.getElementById('chatInput').focus();
}

async function checkChatProvider() {
  try {
    const status = await fetchJSON('/api/provider/status');
    if (!status.active || !status.hasApiKey) {
      const input = document.getElementById('chatInput');
      const sendBtn = document.getElementById('chatSendBtn');
      if (input) {
        input.placeholder = 'AI provider not configured. Go to Settings to set up.';
        input.disabled = true;
      }
      if (sendBtn) sendBtn.disabled = true;
    } else {
      const input = document.getElementById('chatInput');
      const sendBtn = document.getElementById('chatSendBtn');
      if (input) { input.placeholder = 'Ask k8ops AI...'; input.disabled = false; }
      if (sendBtn) sendBtn.disabled = false;
    }
  } catch(e) { /* silent */ }
}

export function closeChatOverlay() {
  document.getElementById('chatOverlay').classList.remove('active');
}

// ESC to close
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && document.getElementById('chatOverlay').classList.contains('active')) {
    closeChatOverlay();
  }
});

export async function loadConversations() {
  try {
    const data = await fetchJSON('/api/chat/conversations');
    const list = document.getElementById('convList');
    if (!data.conversations?.length) {
      list.innerHTML = '<div class="empty">No active conversations</div>';
      return;
    }
    list.innerHTML = `<table><thead><tr><th>ID</th><th>Messages</th><th>Memory</th><th>Tokens</th><th>Updated</th><th></th></tr></thead>
      <tbody>${data.conversations.map(c => `<tr>
        <td style="font-size:12px;">${c.id.substring(0,20)}...</td>
        <td>${c.messageCount}</td>
        <td>${c.memoryCount}</td>
        <td>${c.tokenEstimate}</td>
        <td>${timeAgo(c.updatedAt)}</td>
        <td><button onclick="deleteConv('${c.id}')" style="background:none;border:none;color:#f85149;cursor:pointer;">Delete</button></td>
      </tr>`).join('')}</tbody></table>`;
  } catch(e) { /* ignore */ }
}

export async function deleteConv(id) {
  await fetch('/api/chat/conversations?id=' + id, {method: 'DELETE'});
  loadConversations();
}

export function newConversation() {
  currentConvId = null;
  pendingToolCalls = '';
  document.getElementById('chatMessages').innerHTML =
    '<div style="text-align:center;color:#8b949e;padding:120px 40px;">New conversation started. Ask anything!</div>';
  document.getElementById('chatContext').textContent = '';
  document.getElementById('chatConvInfo').textContent = '';
  document.getElementById('chatInput').focus();
}

export async function sendChatMessage() {
  const input = document.getElementById('chatInput');
  const msg = input.value.trim();
  if (!msg) return;
  input.value = '';
  input.style.height = 'auto';
  input.disabled = true;
  document.getElementById('chatSendBtn').disabled = true;
  document.getElementById('chatSendBtn').textContent = 'Thinking...';

  addChatMessage('user', msg);

  try {
    const resp = await fetch('/api/chat', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({message: msg, conversationId: currentConvId}),
    });

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let assistantDiv = null;
    pendingToolCalls = '';

    while (true) {
      const {done, value} = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, {stream: true});

      const lines = buffer.split('\n');
      buffer = lines.pop();

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        try {
          const event = JSON.parse(line.substring(6));
          if (event.type === 'conversation') {
            currentConvId = event.data.conversationId;
            document.getElementById('chatConvInfo').textContent = 'Conv: ' + currentConvId.substring(0,16);
          } else if (event.type === 'thinking_delta') {
            // Streaming text chunk (could be thinking or final answer)
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');
            // Store the text temporarily - will be finalized as thinking or answer
            let buf = assistantDiv._streamBuf || '';
            buf += event.data.delta;
            assistantDiv._streamBuf = buf;
            // Render as streaming thinking
            let tDiv = assistantDiv.querySelector('.chat-thinking');
            if (!tDiv) {
              tDiv = document.createElement('div');
              tDiv.className = 'chat-thinking expanded streaming';
              tDiv.innerHTML = '<div class="chat-thinking-header" onclick="toggleThinking(this)">' +
                '<span class="chat-thinking-arrow">▶</span>' +
                '<span>Thinking…</span></div>' +
                '<div class="chat-thinking-body md"></div>';
              getToolsArea(assistantDiv).appendChild(tDiv);
            }
            tDiv.querySelector('.chat-thinking-body').innerHTML = renderMarkdown(buf);
            scrollChatBottom();
          } else if (event.type === 'thinking') {
            // Thinking done - collapse it
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');
            let tDiv = assistantDiv.querySelector('.chat-thinking');
            if (tDiv) {
              tDiv.classList.remove('streaming');
              tDiv.classList.remove('expanded');
              tDiv.querySelector('.chat-thinking-header span:last-child').textContent = 'Thinking (click to expand)';
            }
            // If there's a done signal with content, update it
            if (event.data.done && event.data.content) {
              if (tDiv) tDiv.querySelector('.chat-thinking-body').innerHTML = renderMarkdown(event.data.content);
            }
            // Clear stream buffer
            assistantDiv._streamBuf = '';
          } else if (event.type === 'answer_delta') {
            // Not used currently (answer comes from backend as full event)
            // But ready for future true answer streaming
          } else if (event.type === 'tool_call') {
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');
            // Finalize thinking (collapse)
            const tDiv = assistantDiv.querySelector('.chat-thinking');
            if (tDiv) {
              tDiv.classList.remove('streaming');
              tDiv.classList.remove('expanded');
              tDiv.querySelector('.chat-thinking-header span:last-child').textContent = 'Thinking (click to expand)';
            }
            // Add tool line: running state
            const toolLine = document.createElement('div');
            toolLine.className = 'chat-tool-line running';
            toolLine.dataset.toolName = event.data.name;
            toolLine.innerHTML =
              '<span class="chat-tool-icon"><span class="spinner"></span></span>' +
              '<span class="chat-tool-name">' + escapeHtml(event.data.name) + '</span>' +
              '<span class="chat-tool-status running">running…</span>';
            getToolsArea(assistantDiv).appendChild(toolLine);
            scrollChatBottom();
          } else if (event.type === 'tool_result') {
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');
            // Update tool line status
            const toolLines = assistantDiv.querySelectorAll('.chat-tool-line');
            const lastRunning = Array.from(toolLines).reverse().find(l => l.classList.contains('running'));
            if (lastRunning) {
              lastRunning.classList.remove('running');
              const ok = event.data.success;
              const toolName = lastRunning.dataset.toolName || '';
              lastRunning.classList.add(ok ? 'success' : 'failed');
              lastRunning.querySelector('.chat-tool-icon').innerHTML = ok ? '\u2713' : '\u2715';
              lastRunning.querySelector('.chat-tool-status').className = 'chat-tool-status ' + (ok ? 'success' : 'failed');
              lastRunning.querySelector('.chat-tool-status').textContent = ok ? 'ok' : 'failed';
              // Add expandable visualization for tool results
              if (ok && event.data.result) {
                const vizHtml = renderToolResult(toolName, event.data.result);
                if (vizHtml) {
                  lastRunning.style.cursor = 'pointer';
                  lastRunning.addEventListener('click', function() {
                    let detail = lastRunning.nextElementSibling;
                    if (!detail || !detail.classList.contains('tool-result-detail')) {
                      detail = document.createElement('div');
                      detail.className = 'tool-result-detail';
                      detail.innerHTML = vizHtml;
                      lastRunning.parentNode.insertBefore(detail, lastRunning.nextSibling);
                      detail.style.display = 'block';
                    } else {
                      detail.style.display = detail.style.display === 'none' ? 'block' : 'none';
                    }
                  });
                  lastRunning.querySelector('.chat-tool-status').textContent = 'ok \u00B7 click to view';
                }
              }
            }
            scrollChatBottom();
          } else if (event.type === 'memory') {
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');
            appendAssistantContent(assistantDiv, '<div class="chat-memory">' +
              '<strong>Context Compressed</strong> - Older messages summarized into memory</div>');
          } else if (event.type === 'answer') {
            pendingToolCalls = '';
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');

            // The streamed thinking_delta was actually the final answer (no tool calls).
            // Remove the thinking block and render as answer.
            const thinkingBlock = assistantDiv.querySelector('.chat-thinking');
            const streamedText = assistantDiv._streamBuf || '';

            // Remove thinking block since it was actually the answer
            if (thinkingBlock) thinkingBlock.remove();
            assistantDiv._streamBuf = '';

            // Render final answer as markdown
            const html = renderMarkdown(event.data.content || streamedText);
            let ansDiv = assistantDiv.querySelector('.chat-answer');
            if (!ansDiv) {
              ansDiv = document.createElement('div');
              ansDiv.className = 'chat-answer md';
              assistantDiv.querySelector('.chat-text').appendChild(ansDiv);
            }
            ansDiv.innerHTML = html;
            document.getElementById('chatContext').innerHTML =
              '<span style="color:#3fb950;">Steps: ' + event.data.steps + '</span> | ' +
              '<span>Tokens: ' + event.data.totalTokens + '</span> ' +
              '(prompt: ' + event.data.promptTokens + ', completion: ' + event.data.completionTokens + ')';
          } else if (event.type === 'error') {
            if (!assistantDiv) assistantDiv = addChatMessage('assistant', '');
            appendAssistantContent(assistantDiv, '<div style="color:#f85149;">Error: ' + escapeHtml(event.data.message) + '</div>');
          }
        } catch(e) { /* ignore parse errors */ }
      }
    }
  } catch(e) {
    addChatMessage('error', 'Connection error: ' + e.message);
  }

  input.disabled = false;
  document.getElementById('chatSendBtn').disabled = false;
  document.getElementById('chatSendBtn').textContent = 'Send';
  input.focus();
}

export function getToolsArea(div) {
  let area = div.querySelector('.chat-tools');
  if (!area) {
    area = document.createElement('div');
    area.className = 'chat-tools';
    div.querySelector('.chat-text').innerHTML = '';
    div.querySelector('.chat-text').appendChild(area);
  }
  return area;
}

export function toggleThinking(header) {
  const thinking = header.parentElement;
  thinking.classList.toggle('expanded');
}

export function appendAssistantContent(div, content) {
  // Put tool/thinking content in a collapsible area before final answer
  let toolsDiv = div.querySelector('.chat-tools');
  if (!toolsDiv) {
    toolsDiv = document.createElement('div');
    toolsDiv.className = 'chat-tools';
    div.querySelector('.chat-text').innerHTML = '';
    div.querySelector('.chat-text').appendChild(toolsDiv);
  }
  toolsDiv.innerHTML = content;
  scrollChatBottom();
}

export function addChatMessage(role, content) {
  const container = document.getElementById('chatMessages');
  const div = document.createElement('div');
  div.className = 'chat-msg chat-msg-' + role;
  const roleLabel = role === 'user' ? 'You' : role === 'assistant' ? 'AI' : 'System';
  const roleColor = role === 'user' ? '#58a6ff' : role === 'assistant' ? '#3fb950' : '#f85149';

  let contentHtml;
  if (role === 'user') {
    contentHtml = '<div class="md">' + renderMarkdown(content) + '</div>';
  } else {
    contentHtml = '<div class="chat-text"><div class="md">' + renderMarkdown(content) + '</div></div>';
  }

  div.innerHTML = '<div class="chat-msg-label" style="color:' + roleColor + ';">' + roleLabel + '</div>' + contentHtml;
  container.appendChild(div);
  scrollChatBottom();
  return div;
}

export function scrollChatBottom() {
  const c = document.getElementById('chatMessages');
  c.scrollTo({ top: c.scrollHeight, behavior: 'smooth' });
}

export function formatJSON(str) {
  try { return JSON.stringify(JSON.parse(str), null, 2); } catch(e) { return str; }
}

// --- Lightweight Markdown Renderer ---
export function renderMarkdown(md) {
  if (!md) return '';
  // Step 1: escape HTML (prevents XSS from LLM output)
  let text = md.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // Step 2: Extract code blocks first (to protect them from inline formatting)
  const codeBlocks = [];
  text = text.replace(/```(\w*)\n?([\s\S]*?)```/g, (m, lang, code) => {
    const idx = codeBlocks.length;
    const cleaned = code.replace(/\n$/, '');
    const langLabel = lang ? lang.toUpperCase() : 'CODE';
    codeBlocks.push('<div class="code-block-wrapper">'
      + '<div class="code-block-header">'
      + '<span class="code-block-lang">' + escapeHtml(langLabel) + '</span>'
      + '<button class="code-copy-btn" onclick="copyCodeBlock(this)">Copy</button>'
      + '</div>'
      + '<pre><code class="lang-' + escapeHtml(lang) + '">' + cleaned + '</code></pre>'
      + '</div>');
    return '\x00CODEBLOCK' + idx + '\x00';
  });

  // Step 3: Inline code
  const inlineCodes = [];
  text = text.replace(/`([^`]+)`/g, (m, code) => {
    const idx = inlineCodes.length;
    inlineCodes.push('<code>' + code + '</code>');
    return '\x00INLINE' + idx + '\x00';
  });

  // Step 4: Tables
  text = renderMarkdownTables(text);

  // Step 5: Headers
  text = text.replace(/^###\s+(.+)$/gm, '<h3>$1</h3>');
  text = text.replace(/^##\s+(.+)$/gm, '<h2>$1</h2>');
  text = text.replace(/^#\s+(.+)$/gm, '<h1>$1</h1>');

  // Step 6: Bold, italic, strikethrough
  text = text.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>');
  text = text.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  text = text.replace(/__(.+?)__/g, '<strong>$1</strong>');
  text = text.replace(/(?<!\w)\*([^*]+?)\*(?!\w)/g, '<em>$1</em>');
  text = text.replace(/~~(.+?)~~/g, '<del>$1</del>');

  // Step 7: Links (sanitize URL to prevent javascript: and other dangerous protocols)
  text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (m, label, url) => {
    const trimmedUrl = url.trim();
    // Allow only http:, https:, mailto:, and relative URLs
    if (/^(https?:|mailto:|#|\/)/i.test(trimmedUrl)) {
      return '<a href="' + trimmedUrl + '" target="_blank" rel="noopener noreferrer">' + label + '</a>';
    }
    // Block javascript:, data:, vbscript: etc — show label as plain text
    return label;
  });

  // Step 8: Blockquotes
  text = text.replace(/^&gt;\s+(.+)$/gm, '<blockquote>$1</blockquote>');
  text = text.replace(/<\/blockquote>\n<blockquote>/g, '\n');

  // Step 9: Horizontal rules
  text = text.replace(/^---+$/gm, '<hr>');

  // Step 10: Lists (unordered)
  text = text.replace(/^(\s*)[-*+]\s+(.+)$/gm, '$1<li>$2</li>');
  text = text.replace(/(<li>[\s\S]*?<\/li>)(?!\s*<li>)/g, '<ul>$1</ul>');
  text = text.replace(/<\/li>\n<ul>/g, '</li>\n');

  // Step 11: Ordered lists
  text = text.replace(/^\d+\.\s+(.+)$/gm, '<oli>$1</oli>');
  text = text.replace(/(<oli>[\s\S]*?<\/oli>)(?!\s*<oli>)/g, '<ol>$1</ol>');
  text = text.replace(/<\/oli>/g, '</li>').replace(/<oli>/g, '<li>');

  // Step 12: Paragraphs (double newline = paragraph break)
  text = text.replace(/\n\n+/g, '\n\n');
  const paragraphs = text.split('\n\n');
  text = paragraphs.map(p => {
    p = p.trim();
    if (!p) return '';
    if (/^<(h[1-6]|ul|ol|pre|blockquote|hr|table|div)/.test(p)) return p;
    return '<p>' + p.replace(/\n/g, '<br>') + '</p>';
  }).join('\n');

  // Step 13: Restore code blocks
  text = text.replace(/\x00CODEBLOCK(\d+)\x00/g, (m, idx) => codeBlocks[parseInt(idx)]);
  text = text.replace(/\x00INLINE(\d+)\x00/g, (m, idx) => inlineCodes[parseInt(idx)]);

  // Step 14: Enhance kubectl/shell code blocks with action cards
  text = text.replace(/<div class="code-block-wrapper">([\s\S]*?)<pre><code[^>]*>([\s\S]*?)<\/code><\/pre>\s*<\/div>/g, (match, header, code) => {
    const cmd = code.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').trim();
    // Only add action buttons for shell/kubectl commands
    if (/^(kubectl|helm|docker|crictl|ctr|k3s|istioctl)\s/.test(cmd) || /^\$\s/.test(cmd)) {
      const cleanCmd = cmd.replace(/^\$\s*/, '');
      const runBtn = '<button class="action-card-btn action-run-btn" onclick="runSuggestedCommand(this)" data-cmd="' + escapeHtml(cleanCmd) + '">&#9654; Run in Chat</button>';
      const copyBtn = '<button class="action-card-btn action-copy-cmd" onclick="copyToClipboard(this)" data-cmd="' + escapeHtml(cleanCmd) + '">&#128203; Copy</button>';
      return match + '<div class="action-card">' + runBtn + copyBtn + '</div>';
    }
    return match;
  });

  return text;
}

export function renderMarkdownTables(text) {
  // Simple table: header row | header row\n---|---\n| cell | cell |
  const lines = text.split('\n');
  let result = [];
  let i = 0;
  while (i < lines.length) {
    // Check if current and next line form a table header
    if (i + 1 < lines.length &&
        lines[i].includes('|') &&
        /^\|?[\s-:|]+\|[\s-:|]*$/.test(lines[i+1].trim())) {
      const headerCells = lines[i].split('|').map(c => c.trim()).filter(c => c !== '');
      let html = '<table><thead><tr>';
      headerCells.forEach(c => html += '<th>' + c + '</th>');
      html += '</tr></thead><tbody>';
      i += 2; // skip header and separator
      while (i < lines.length && lines[i].includes('|') && lines[i].trim()) {
        const cells = lines[i].split('|').map(c => c.trim());
        // Remove empty first/last from leading/trailing |
        if (cells[0] === '') cells.shift();
        if (cells[cells.length-1] === '') cells.pop();
        html += '<tr>';
        cells.forEach(c => html += '<td>' + c + '</td>');
        html += '</tr>';
        i++;
      }
      html += '</tbody></table>';
      result.push(html);
    } else {
      result.push(lines[i]);
      i++;
    }
  }
  return result.join('\n');
}

export function copyCodeBlock(btn) {
  const wrapper = btn.closest('.code-block-wrapper');
  if (!wrapper) return;
  const codeEl = wrapper.querySelector('code');
  if (!codeEl) return;
  const text = codeEl.textContent;
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(function() {
      btn.textContent = 'Copied!';
      setTimeout(function() { btn.textContent = 'Copy'; }, 2000);
    }).catch(function() {
      fallbackCopy(text, btn);
    });
  } else {
    fallbackCopy(text, btn);
  }
}

export function fallbackCopy(text, btn) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.select();
  try { document.execCommand('copy'); btn.textContent = 'Copied!'; setTimeout(function() { btn.textContent = 'Copy'; }, 2000); } catch(e) {}
  document.body.removeChild(ta);
}

// ============================
// AI Diagnostic Action Cards
// ============================

// Run a suggested kubectl command by placing it in the chat input
export function runSuggestedCommand(btn) {
  const cmd = btn.getAttribute('data-cmd');
  if (!cmd) return;
  const input = document.getElementById('chatInput');
  if (input) {
    input.value = cmd;
    input.focus();
    // Visual feedback
    btn.style.background = '#3fb950';
    btn.textContent = '✓ Loaded!';
    setTimeout(function() {
      btn.style.background = '';
      btn.innerHTML = '&#9654; Run in Chat';
    }, 1500);
  }
}

// Copy suggested command to clipboard
export function copyToClipboard(btn) {
  const cmd = btn.getAttribute('data-cmd');
  if (!cmd) return;
  const ta = document.createElement('textarea');
  ta.value = cmd;
  ta.style.position = 'fixed';
  ta.style.opacity = '0';
  document.body.appendChild(ta);
  ta.select();
  try {
    document.execCommand('copy');
    btn.textContent = '✓ Copied!';
    setTimeout(function() { btn.innerHTML = '&#128203; Copy'; }, 2000);
  } catch(e) {}
  document.body.removeChild(ta);
}

// ============================
// Tool Result Visualization
// ============================

// Render AI tool results as formatted tables instead of raw JSON
function renderToolResult(toolName, result) {
  if (!result) return '';
  let data;
  try {
    data = typeof result === 'string' ? JSON.parse(result) : result;
  } catch(e) {
    return '<pre class="tool-raw">' + escapeHtml(String(result).substring(0, 2000)) + '</pre>';
  }

  // Pod list visualization
  if (data.items && Array.isArray(data.items) && data.items.length > 0) {
    return renderItemsTable(data.items, toolName);
  }

  // Node list or single resource
  if (data.nodes && Array.isArray(data.nodes)) {
    return renderNodesTable(data.nodes);
  }

  // Single pod/resource detail
  if (data.metadata && data.metadata.name) {
    return renderResourceDetail(data);
  }

  // Generic key-value
  if (typeof data === 'object' && Object.keys(data).length > 0) {
    return renderKeyValue(data);
  }

  return '<pre class="tool-raw">' + escapeHtml(JSON.stringify(data, null, 2).substring(0, 2000)) + '</pre>';
}

function renderItemsTable(items, toolName) {
  // Detect if items look like pods
  const looksLikePods = items.some(function(i) { return i.phase || i.status || (i.metadata && i.metadata.name); });
  if (looksLikePods && (toolName.includes('pod') || toolName.includes('Pod'))) {
    return '<div class="tool-table-wrapper">' +
      '<div class="tool-table-title">Pods (' + items.length + ')</div>' +
      '<table class="tool-table">' +
      '<thead><tr><th>Name</th><th>Namespace</th><th>Phase</th><th>Restarts</th><th>Node</th></tr></thead>' +
      '<tbody>' + items.slice(0, 50).map(function(p) {
        var name = p.name || (p.metadata && p.metadata.name) || '-';
        var ns = p.namespace || (p.metadata && p.metadata.namespace) || '-';
        var phase = p.phase || (p.status && p.status.phase) || '-';
        var restarts = p.restarts !== undefined ? p.restarts : '-';
        var node = p.node || (p.spec && p.spec.nodeName) || '-';
        var phaseColor = phase === 'Running' ? '#3fb950' : phase === 'Pending' ? '#d29922' : phase === 'Failed' ? '#f85149' : '#8b949e';
        return '<tr><td style="font-family:monospace;font-size:11px;">' + escapeHtml(String(name).substring(0, 40)) + '</td>' +
          '<td style="font-size:11px;">' + escapeHtml(ns) + '</td>' +
          '<td><span style="color:' + phaseColor + ';">' + escapeHtml(phase) + '</span></td>' +
          '<td>' + (restarts > 3 ? '<span style="color:#f85149;">' + restarts + '</span>' : restarts) + '</td>' +
          '<td style="font-size:11px;color:#8b949e;">' + escapeHtml(String(node).substring(0, 25)) + '</td></tr>';
      }).join('') + '</tbody></table>' +
      (items.length > 50 ? '<div class="tool-table-footer">Showing 50 of ' + items.length + '</div>' : '') +
    '</div>';
  }

  // Generic items table
  var keys = Object.keys(items[0] || {});
  return '<div class="tool-table-wrapper">' +
    '<div class="tool-table-title">Results (' + items.length + ')</div>' +
    '<table class="tool-table"><thead><tr>' +
    keys.slice(0, 6).map(function(k) { return '<th>' + escapeHtml(k) + '</th>'; }).join('') +
    '</tr></thead><tbody>' +
    items.slice(0, 30).map(function(item) {
      return '<tr>' + keys.slice(0, 6).map(function(k) {
        var v = item[k];
        if (v == null) return '<td>-</td>';
        if (typeof v === 'object') v = JSON.stringify(v);
        return '<td style="font-size:11px;max-width:180px;overflow:hidden;text-overflow:ellipsis;">' + escapeHtml(String(v).substring(0, 60)) + '</td>';
      }).join('') + '</tr>';
    }).join('') +
    '</tbody></table></div>';
}

function renderNodesTable(nodes) {
  return '<div class="tool-table-wrapper">' +
    '<div class="tool-table-title">Nodes (' + nodes.length + ')</div>' +
    '<table class="tool-table">' +
    '<thead><tr><th>Name</th><th>Status</th><th>CPU</th><th>Memory</th><th>Pods</th></tr></thead>' +
    '<tbody>' + nodes.map(function(n) {
      var name = n.name || n.Name || '-';
      var status = n.status || n.Status || '-';
      var cpu = n.cpuRequestedPct || n.CPU || '-';
      var mem = n.memRequestedPct || n.Memory || '-';
      var pods = n.podCount || n.Pods || '-';
      var statusColor = status === 'Ready' ? '#3fb950' : '#f85149';
      return '<tr><td style="font-family:monospace;font-size:11px;">' + escapeHtml(String(name).substring(0, 30)) + '</td>' +
        '<td><span style="color:' + statusColor + ';">' + escapeHtml(status) + '</span></td>' +
        '<td>' + (typeof cpu === 'number' ? cpu + '%' : cpu) + '</td>' +
        '<td>' + (typeof mem === 'number' ? mem + '%' : mem) + '</td>' +
        '<td>' + pods + '</td></tr>';
    }).join('') + '</tbody></table></div>';
}

function renderResourceDetail(data) {
  var meta = data.metadata || {};
  var rows = [
    ['Name', meta.name],
    ['Namespace', meta.namespace],
    ['Kind', data.kind || data.Kind],
    ['Created', meta.creationTimestamp],
    ['API Version', data.apiVersion],
  ];
  if (data.spec) {
    Object.keys(data.spec).slice(0, 5).forEach(function(k) {
      var v = data.spec[k];
      if (typeof v === 'string' || typeof v === 'number') rows.push([k, v]);
    });
  }
  return '<div class="tool-table-wrapper">' +
    '<div class="tool-table-title">' + escapeHtml(data.kind || 'Resource') + '/' + escapeHtml(meta.name || '') + '</div>' +
    '<table class="tool-table"><tbody>' +
    rows.filter(function(r) { return r[1] != null && r[1] !== ''; }).map(function(r) {
      return '<tr><th style="width:120px;text-align:right;">' + escapeHtml(r[0]) + '</th><td>' + escapeHtml(String(r[1])) + '</td></tr>';
    }).join('') +
    '</tbody></table></div>';
}

function renderKeyValue(data) {
  var keys = Object.keys(data).slice(0, 15);
  return '<div class="tool-table-wrapper">' +
    '<table class="tool-table"><tbody>' +
    keys.map(function(k) {
      var v = data[k];
      if (v == null) v = '-';
      if (typeof v === 'object') v = JSON.stringify(v).substring(0, 120);
      return '<tr><th style="width:130px;text-align:right;">' + escapeHtml(k) + '</th><td>' + escapeHtml(String(v)) + '</td></tr>';
    }).join('') +
    '</tbody></table></div>';
}

