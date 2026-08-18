// toast.ts - Decoupled visual notification system

export function showToast(message: string, type: 'success' | 'error' = 'success'): void {
  const toaster = document.getElementById('toast-toaster');
  if (!toaster) return;
  
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  
  const dot = document.createElement('span');
  dot.className = 'status-dot';
  const color = type === 'success' ? 'var(--neon-green)' : 'var(--neon-red)';
  dot.style.backgroundColor = color;
  dot.style.boxShadow = `0 0 8px ${color}`;

  const textSpan = document.createElement('span');
  textSpan.textContent = message;

  toast.appendChild(dot);
  toast.appendChild(textSpan);
  toast.style.cursor = 'pointer';

  // Allow click-to-dismiss immediately for all toasts
  toast.addEventListener('click', () => {
    toast.classList.remove('active');
    setTimeout(() => toast.remove(), 400);
  });

  toaster.appendChild(toast);
  
  setTimeout(() => toast.classList.add('active'), 50);
  
  // Auto-dismiss only success toasts; keep error toasts visible until clicked
  if (type === 'success') {
    setTimeout(() => {
      if (toast.parentElement) {
        toast.classList.remove('active');
        setTimeout(() => toast.remove(), 400);
      }
    }, 4000);
  }
}

// Bind to window for global access across dynamic ES6 modules
(window as any).showToast = showToast;
