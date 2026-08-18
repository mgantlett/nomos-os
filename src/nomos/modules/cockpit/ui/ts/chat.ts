// chat.ts - Swarm Chat Drawer UI module

export function openSwarmChatDrawer(): void {
  const drawer = document.getElementById('swarm-chat-drawer');
  const overlay = document.getElementById('drawer-overlay');
  if (drawer && overlay) {
    drawer.classList.add('open');
    overlay.classList.add('open');
    // Close other drawers to prevent overlapping UI
    const taskDrawer = document.getElementById('task-details-drawer');
    if (taskDrawer) taskDrawer.classList.remove('open');
    const swarmDrawer = document.getElementById('swarm-console-drawer');
    if (swarmDrawer) swarmDrawer.classList.remove('open');

    // Autofocus input robustly
    const chatInputText = document.getElementById('chat-input-text') as HTMLTextAreaElement;
    if (chatInputText) {
      chatInputText.focus();
      setTimeout(() => chatInputText.focus(), 100);
      setTimeout(() => chatInputText.focus(), 300);
    }
  }
}

export function closeSwarmChatDrawer(): void {
  const drawer = document.getElementById('swarm-chat-drawer');
  const overlay = document.getElementById('drawer-overlay');
  if (drawer) drawer.classList.remove('open');
  const taskDrawer = document.getElementById('task-details-drawer');
  const swarmDrawer = document.getElementById('swarm-console-drawer');
  const taskOpen = taskDrawer && taskDrawer.classList.contains('open');
  const swarmOpen = swarmDrawer && swarmDrawer.classList.contains('open');
  if (!taskOpen && !swarmOpen && overlay) {
    overlay.classList.remove('open');
  }
}

export function parseInlineMarkdown(container: HTMLElement, text: string): void {
  const regex = /(\*\*.*?\*\*|`.*?`|\[.*?\]\(.*?\))/g;
  const parts = text.split(regex);
  
  for (const part of parts) {
    if (part.startsWith('**') && part.endsWith('**')) {
      const strong = document.createElement('strong');
      strong.textContent = part.slice(2, -2);
      container.appendChild(strong);
    } else if (part.startsWith('`') && part.endsWith('`')) {
      const code = document.createElement('code');
      code.textContent = part.slice(1, -1);
      container.appendChild(code);
    } else if (part.startsWith('[') && part.includes('](') && part.endsWith(')')) {
      const openBracket = part.indexOf('[');
      const closeBracket = part.indexOf(']');
      const openParen = part.indexOf('(');
      const closeParen = part.indexOf(')');
      if (openBracket !== -1 && closeBracket > openBracket && openParen > closeBracket && closeParen > openParen) {
        const linkText = part.substring(openBracket + 1, closeBracket);
        const linkUrl = part.substring(openParen + 1, closeParen);
        const a = document.createElement('a');
        a.textContent = linkText;
        a.href = linkUrl;
        a.target = '_blank';
        a.rel = 'noopener noreferrer';
        a.style.color = '#58a6ff'; // Nice clean blue link color matching dark theme
        a.style.textDecoration = 'underline';
        container.appendChild(a);
      } else {
        container.appendChild(document.createTextNode(part));
      }
    } else {
      container.appendChild(document.createTextNode(part));
    }
  }
}

export function renderBubbleText(bubble: HTMLElement, text: string): void {
  bubble.replaceChildren();
  const preParts = text.split('```');
  for (let i = 0; i < preParts.length; i++) {
    if (i % 2 === 1) {
      const pre = document.createElement('pre');
      const code = document.createElement('code');
      const rawContent = preParts[i];
      const firstNewline = rawContent.indexOf('\n');
      if (firstNewline !== -1) {
        const lang = rawContent.substring(0, firstNewline).trim();
        if (lang.length > 0 && lang.length < 15 && /^[a-zA-Z0-9_-]+$/.test(lang)) {
          code.className = `language-${lang}`;
          code.textContent = rawContent.substring(firstNewline + 1);
        } else {
          code.textContent = rawContent;
        }
      } else {
        code.textContent = rawContent;
      }
      pre.appendChild(code);
      bubble.appendChild(pre);
    } else {
      const lines = preParts[i].split('\n');
      let currentList: HTMLUListElement | null = null;
      let currentOl: HTMLOListElement | null = null;

      for (let j = 0; j < lines.length; j++) {
        const line = lines[j];
        const trimmed = line.trim();

        if (!trimmed) {
          currentList = null;
          currentOl = null;
          continue;
        }

        // Headers: #, ##, ###, ####, #####, ######
        if (trimmed.startsWith('#')) {
          currentList = null;
          currentOl = null;
          let level = 0;
          while (level < trimmed.length && trimmed[level] === '#') {
            level++;
          }
          const headerText = trimmed.substring(level).trim();
          if (level <= 6) {
            const h = document.createElement(`h${level}`);
            h.style.marginTop = '0.5rem';
            h.style.marginBottom = '0.25rem';
            h.style.fontWeight = 'bold';
            parseInlineMarkdown(h, headerText);
            bubble.appendChild(h);
            continue;
          }
        }

        // Unordered lists: - item, * item
        if (trimmed.startsWith('- ') || trimmed.startsWith('* ')) {
          currentOl = null;
          if (!currentList) {
            currentList = document.createElement('ul');
            currentList.style.listStyleType = 'disc';
            currentList.style.paddingLeft = '1.25rem';
            currentList.style.margin = '0.25rem 0';
            bubble.appendChild(currentList);
          }
          const li = document.createElement('li');
          parseInlineMarkdown(li, trimmed.substring(2));
          currentList.appendChild(li);
          continue;
        }

        // Ordered lists: 1. item, 2. item
        const numMatch = trimmed.match(/^(\d+)\.\s+(.*)$/);
        if (numMatch) {
          currentList = null;
          if (!currentOl) {
            currentOl = document.createElement('ol');
            currentOl.style.listStyleType = 'decimal';
            currentOl.style.paddingLeft = '1.25rem';
            currentOl.style.margin = '0.25rem 0';
            bubble.appendChild(currentOl);
          }
          const li = document.createElement('li');
          parseInlineMarkdown(li, numMatch[2]);
          currentOl.appendChild(li);
          continue;
        }

        // Plain paragraph
        currentList = null;
        currentOl = null;
        const p = document.createElement('p');
        p.style.margin = '0.25rem 0';
        parseInlineMarkdown(p, line);
        bubble.appendChild(p);
      }
    }
  }
}

export function clearChatMessages(): void {
  const container = document.getElementById('chat-messages-container');
  if (container) {
    container.replaceChildren();
  }
}

export function renderChatMessage(sender: 'user' | 'agent' | 'system', text: string): void {
  const container = document.getElementById('chat-messages-container');
  if (!container) return;

  const row = document.createElement('div');
  row.className = `chat-msg-row ${sender}`;

  const bubble = document.createElement('div');
  bubble.className = 'chat-bubble';

  if (sender === 'system') {
    bubble.textContent = text;
  } else {
    renderBubbleText(bubble, text);
  }

  row.appendChild(bubble);
  container.appendChild(row);
  container.scrollTop = container.scrollHeight;
}

export function sendUserChatMessage(): void {
  const inputEl = document.getElementById('chat-input-text') as HTMLTextAreaElement;
  const sendBtn = document.getElementById('btn-chat-send') as HTMLButtonElement;
  if (!inputEl || !sendBtn) return;

  const text = inputEl.value.trim();
  if (!text) return;

  renderChatMessage('user', text);
  inputEl.value = '';
  inputEl.style.height = 'auto'; // Reset height
  sendBtn.disabled = true;
  inputEl.disabled = true;

  const container = document.getElementById('chat-messages-container');
  if (!container) return;

  const row = document.createElement('div');
  row.className = 'chat-msg-row agent';
  const bubble = document.createElement('div');
  bubble.className = 'chat-bubble';
  
  const loading = document.createElement('div');
  loading.className = 'chat-loading-dots';
  for (let i = 0; i < 3; i++) {
    loading.appendChild(document.createElement('span'));
  }
  bubble.appendChild(loading);
  row.appendChild(bubble);
  container.appendChild(row);
  container.scrollTop = container.scrollHeight;

  // Start async streaming fetch from backend conduit
  (async () => {
    try {
      const response = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          prompt: text,
          contextPath: undefined,
          contextName: undefined
        })
      });

      if (!response.ok) {
        throw new Error(`HTTP error ${response.status}`);
      }

      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error('Readable stream not supported');
      }

      const decoder = new TextDecoder('utf-8');
      let accumulatedText = '';
      let isFirstChunk = true;

      // Safe DOM-based markdown formatter to prevent XSS
      const updateBubbleContent = (content: string) => {
        renderBubbleText(bubble, content);
      };

      while (true) {
        const { value, done } = await reader.read();
        if (done) break;

        const textChunk = decoder.decode(value, { stream: true });
        const lines = textChunk.split('\n');
        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed) continue;
          if (trimmed.startsWith('data: ')) {
            const dataStr = trimmed.substring(6).trim();
            if (dataStr === '[DONE]') {
              break;
            }
            try {
              const dataObj = JSON.parse(dataStr);
              const content = dataObj.choices?.[0]?.delta?.content || '';
              if (content) {
                if (isFirstChunk) {
                  bubble.replaceChildren(); // remove loading dots
                  isFirstChunk = false;
                }
                accumulatedText += content;
                updateBubbleContent(accumulatedText);
                container.scrollTop = container.scrollHeight;
              }
            } catch (err) {
              // ignore partial JSON parse errors
            }
          }
        }
      }
    } catch (err: any) {
      bubble.replaceChildren();
      const p = document.createElement('p');
      p.style.color = 'var(--neon-red)';
      p.textContent = `Error: ${err.message}`;
      bubble.appendChild(p);
    } finally {
      sendBtn.disabled = false;
      inputEl.disabled = false;
      inputEl.focus();
    }
  })();
}
