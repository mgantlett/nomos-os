// artifacts.ts - Decoupled active memory artifact loading, saving, and markdown formatting

import { showToast } from './toast.js';

export let currentActiveArtifactType = 'implementation_plan';

export function getCurrentActiveArtifactType(): string {
  return currentActiveArtifactType;
}

export function setCurrentActiveArtifactType(type: string): void {
  currentActiveArtifactType = type;
}

// Safe HTML String Sanitizer to protect against XSS
export function sanitizeHTMLString(html: string): string {
  return html.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
             .replace(/on\w+="[^"]*"/g, '')
             .replace(/on\w+='[^']*'/g, '')
             .replace(/javascript:[^"']*/g, '');
}

// Render markdown and process Mermaid sequence flows client-side
export function parseLiveMarkdown(val: string): void {
  const previewPane = document.getElementById('spec-preview-body-div');
  if (!previewPane) return;
  if (!val) {
    previewPane.innerHTML = `<div style="color: var(--text-muted); font-size: 0.85rem; padding: 1rem;">Workspace specification is empty. Click here to edit!</div>`; // safe
    return;
  }
  
  const markedObj = (window as any).marked;
  if (markedObj) {
    try {
      const parsed = markedObj.parse(val);
      previewPane.innerHTML = sanitizeHTMLString(parsed); // safe
      const mermaidObj = (window as any).mermaid;
      if (mermaidObj) {
        const codeBlocks = previewPane.querySelectorAll('pre code.language-mermaid');
        codeBlocks.forEach((codeEl) => {
          const preEl = codeEl.parentElement;
          if (preEl) {
            const div = document.createElement('div');
            div.className = 'mermaid';
            div.textContent = codeEl.textContent;
            preEl.replaceWith(div);
          }
        });
        if (typeof mermaidObj.run === 'function') {
          mermaidObj.run({
            nodes: previewPane.querySelectorAll('.mermaid')
          }).catch((err: any) => console.error('Mermaid run error:', err));
        } else if (typeof mermaidObj.init === 'function') {
          mermaidObj.init(undefined, previewPane.querySelectorAll('.mermaid'));
        } else {
          mermaidObj.contentLoaded();
        }
      }
    } catch (err: any) {
      previewPane.innerHTML = `<div style="color: var(--neon-red); font-size: 0.85rem; padding: 1rem;">Rendering Error: ${err.message}</div>`; // safe
    }
  } else {
    const pre = document.createElement('pre');
    pre.style.whiteSpace = 'pre-wrap';
    pre.textContent = val;
    previewPane.replaceChildren(pre);
  }
}

// Fetch artifact content from backend gateway
export function fetchActiveArtifact(type: string): void {
  currentActiveArtifactType = type;
  const selectEl = document.getElementById('artifact-type-select') as HTMLSelectElement | null;
  if (selectEl) {
    selectEl.value = type;
  }
  fetch(`/api/artifacts/${type}`)
    .then(res => res.json())
    .then(data => {
      if (data.error === 'No active task ID found') {
        const textarea = document.getElementById('spec-markdown-textarea') as HTMLTextAreaElement | null;
        if (textarea) {
          textarea.value = '';
        }
        const previewPane = document.getElementById('spec-preview-body-div');
        if (previewPane) {
          previewPane.replaceChildren();
          const div = document.createElement('div');
          div.style.color = 'var(--text-muted)';
          div.style.fontSize = '0.85rem';
          div.style.padding = '1rem';
          
          const text1 = document.createTextNode('No active task. Select a task card and start a task (e.g., via chat command ');
          const codeEl = document.createElement('code');
          codeEl.textContent = '/start <id>';
          const text2 = document.createTextNode(') to edit specifications.');
          
          div.appendChild(text1);
          div.appendChild(codeEl);
          div.appendChild(text2);
          previewPane.appendChild(div);
        }
        return;
      }
      
      if (data.error) {
        throw new Error(data.error);
      }

      const textarea = document.getElementById('spec-markdown-textarea') as HTMLTextAreaElement | null;
      if (textarea) {
        textarea.value = data.content || '';
      }
      parseLiveMarkdown(data.content || '');
    })
    .catch(err => {
      showToast(`Error loading specification: ${err.message}`, 'error');
    });
}

// Save artifact content to backend gateway
export function saveActiveArtifact(): void {
  const textarea = document.getElementById('spec-markdown-textarea') as HTMLTextAreaElement | null;
  if (!textarea) return;
  const content = textarea.value;
  
  fetch(`/api/artifacts/${currentActiveArtifactType}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content })
  })
    .then(res => {
      if (!res.ok) throw new Error('Save failed');
      return res.json();
    })
    .then(() => {
      showToast(`Specification saved successfully!`, 'success');
      parseLiveMarkdown(content);
    })
    .catch(err => {
      showToast(`Failed to save spec: ${err.message}`, 'error');
    });
}

export function triggerPhaseTransition(targetPhase: string, refreshDataCallback: () => Promise<void>): void {
  fetch('/api/phase', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ phase: targetPhase })
  })
    .then(res => {
      if (!res.ok) throw new Error('Phase transition rejected');
      return res.json();
    })
    .then(data => {
      if (data.success) {
        showToast(`Workspace transitioned successfully to ${targetPhase}`, 'success');
        refreshDataCallback();
      } else {
        showToast(`Transition failed: ${data.error || 'rejected'}`, 'error');
      }
    })
    .catch(err => {
      showToast(`Phase transition network error: ${err.message}`, 'error');
    });
}

// Task Details Drawer Markdown Formatter Implementation
export function formatMarkdown(markdown: string): string {
  if (!markdown) return '';
  // Convert headings
  let html = markdown
    .replace(/^### (.*$)/gim, '<h4 style="color:var(--neon-blue); margin-top:1.2rem; margin-bottom:0.4rem;">$1</h4>')
    .replace(/^## (.*$)/gim, '<h3 style="color:var(--neon-blue); margin-top:1.5rem; margin-bottom:0.6rem; border-bottom:1px solid rgba(255,255,255,0.1); padding-bottom:4px;">$1</h3>')
    .replace(/^# (.*$)/gim, '<h2 style="color:var(--neon-blue); margin-top:1.8rem; margin-bottom:0.8rem;">$1</h2>');
  
  // Convert bold and italics
  html = html.replace(/\*\*(.*?)\*\*/g, '<strong style="color:var(--text-main);">$1</strong>');
  html = html.replace(/\*(.*?)\*/g, '<em>$1</em>');
  html = html.replace(/`(.*?)`/g, '<code style="background:rgba(255,255,255,0.1); padding:2px 6px; border-radius:4px; font-family:monospace; color:var(--neon-purple);">$1</code>');

  // Convert checklists
  html = html.replace(/^\s*-\s*\[\s*\]\s*(.*$)/gim, '<div style="margin: 0.4rem 0; display:flex; align-items:flex-start; gap:8px;"><span style="color:var(--text-muted); font-weight:bold; cursor:default;">☐</span><span>$1</span></div>');
  html = html.replace(/^\s*-\s*\[\s*x\s*\]\s*(.*$)/gim, '<div style="margin: 0.4rem 0; display:flex; align-items:flex-start; gap:8px;"><span style="color:var(--neon-green); font-weight:bold; cursor:default;">☑</span><span style="text-decoration:line-through; color:var(--text-muted);">$1</span></div>');

  // Convert bullet lists (if not checklists)
  html = html.replace(/^\s*[-*]\s*(?!(?:\[\s*\]|\[\s*x\s*\]))\s*(.*$)/gim, '<div style="margin: 0.3rem 0 0.3rem 0.8rem; display:flex; align-items:center; gap:6px;"><span style="color:var(--neon-blue); font-size:0.6rem;">●</span><span>$1</span></div>');

  // Convert line breaks / paragraphs
  html = html.split('\n').map(line => {
    const trimmed = line.trim();
    if (trimmed.startsWith('<h') || trimmed.startsWith('<div') || trimmed.startsWith('<ul') || trimmed === '') {
      return line;
    }
    return `<p style="margin: 0.6rem 0; line-height: 1.5; color: var(--text-main, rgba(255,255,255,0.85));">${line}</p>`;
  }).join('\n');

  return html;
}
