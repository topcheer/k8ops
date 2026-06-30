// --- Chat ---
let currentConvId = null;
let pendingToolCalls = ''; // accumulate tool calls before final answer

function openChatOverlay() {
  document.getElementById('chatOverlay').classList.add('active');
  if (!currentConvId) {
    document.getElementById('chatMessages').innerHTML =
      '<div style="text-align:center;color:#8b949e;padding:120px 40px;">Start a conversation with k8ops AI</div>';
  }
  document.getElementById('chatInput').focus();
}

function closeChatOverlay() {
  document.getElementById('chatOverlay').classList.remove('active');
}

// ESC to close
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && document.getElementById('chatOverlay').classList.contains('active')) {
    closeChatOverlay();
  }
});

async function loadConversations() {
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

async function deleteConv(id) {
  await fetch('/api/chat/conversations?id=' + id, {method: 'DELETE'});
  loadConversations();
}

function newConversation() {
  currentConvId = null;
  pendingToolCalls = '';
  document.getElementById('chatMessages').innerHTML =
    '<div style="text-align:center;color:#8b949e;padding:120px 40px;">New conversation started. Ask anything!</div>';
  document.getElementById('chatContext').textContent = '';
  document.getElementById('chatConvInfo').textContent = '';
  document.getElementById('chatInput').focus();
}

async function sendChatMessage() {
  const input = document.getElementById('chatInput');
  const msg = input.value.trim();
  if (!msg) return;
  input.value = '';
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
              lastRunning.classList.add(ok ? 'success' : 'failed');
              lastRunning.querySelector('.chat-tool-icon').innerHTML = ok ? '✓' : '✕';
              lastRunning.querySelector('.chat-tool-status').className = 'chat-tool-status ' + (ok ? 'success' : 'failed');
              lastRunning.querySelector('.chat-tool-status').textContent = ok ? 'ok' : 'failed';
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

function getToolsArea(div) {
  let area = div.querySelector('.chat-tools');
  if (!area) {
    area = document.createElement('div');
    area.className = 'chat-tools';
    div.querySelector('.chat-text').innerHTML = '';
    div.querySelector('.chat-text').appendChild(area);
  }
  return area;
}

function toggleThinking(header) {
  const thinking = header.parentElement;
  thinking.classList.toggle('expanded');
}

function appendAssistantContent(div, content) {
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

function addChatMessage(role, content) {
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

function scrollChatBottom() {
  const c = document.getElementById('chatMessages');
  c.scrollTo({ top: c.scrollHeight, behavior: 'smooth' });
}

function formatJSON(str) {
  try { return JSON.stringify(JSON.parse(str), null, 2); } catch(e) { return str; }
}

// --- Lightweight Markdown Renderer ---
function renderMarkdown(md) {
  if (!md) return '';
  // Step 1: escape HTML
  let text = md.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  // Step 2: Extract code blocks first (to protect them from inline formatting)
  const codeBlocks = [];
  text = text.replace(/```(\w*)\n?([\s\S]*?)```/g, (m, lang, code) => {
    const idx = codeBlocks.length;
    codeBlocks.push('<pre><code class="lang-' + lang + '">' + code.replace(/\n$/, '') + '</code></pre>');
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

  // Step 7: Links
  text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');

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

  return text;
}

function renderMarkdownTables(text) {
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

function escapeHtml(s) {
  if (!s) return '';
  return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

