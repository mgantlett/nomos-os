// ansi.ts - ANSI Escape Code Parser to styled HTML Elements
// Decodes terminal color codes and formatting attributes into styled DOM nodes.

// parseAnsiLine reads a raw log string, extracts ANSI color sequences,
// and returns a collection of styled HTMLSpanElement objects representing the formatted line.
export function parseAnsiLine(text: string): HTMLSpanElement[] {
  // Regex matches standard SGR (Select Graphic Rendition) parameters (e.g. \u001b[31;1m)
  const ansiRegex = /\u001b\[([0-9;]+)m/g;
  const elements: HTMLSpanElement[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  let currentStyle: { color?: string; fontWeight?: string } = {};

  // Use raw text since textContent handles DOM safety natively without double-escaping.
  // This bypasses entity replacement to support correct terminal representation in UI logs.
  const escapedText = text;

  // Iterate over all ANSI escape sequence matches found in the text line
  while ((match = ansiRegex.exec(escapedText)) !== null) {
    // Push prior plain text (if any) under the current style
    const plainText = escapedText.substring(lastIndex, match.index);
    if (plainText) {
      const node = document.createElement('span');
      node.textContent = plainText;
      applyStyle(node, currentStyle);
      elements.push(node);
    }

    // Parse ansi codes to update current style configuration
    const codes = match[1].split(';');
    for (const code of codes) {
      if (code === '0') {
        // Reset all styles back to default
        currentStyle = {};
      } else if (code === '31' || code === '91') {
        // Red color code - map to neon-red styling token
        currentStyle.color = 'var(--neon-red)';
        currentStyle.fontWeight = '700';
      } else if (code === '32' || code === '92') {
        // Green color code - map to neon-green styling token
        currentStyle.color = 'var(--neon-green)';
        currentStyle.fontWeight = '700';
      } else if (code === '33' || code === '93') {
        // Yellow color code - map to neon-yellow styling token
        currentStyle.color = 'var(--neon-yellow)';
      } else if (code === '34' || code === '94') {
        // Blue color code - map to neon-blue styling token
        currentStyle.color = 'var(--neon-blue)';
      } else if (code === '35' || code === '95') {
        // Pink color code - map to neon-pink styling token
        currentStyle.color = 'var(--neon-pink)';
      } else if (code === '36' || code === '96') {
        // Cyan color code - map to hex value styling token
        currentStyle.color = '#06b6d4';
      } else if (code === '1') {
        // Bold attribute - map to bold font weight
        currentStyle.fontWeight = 'bold';
      }
    }

    lastIndex = ansiRegex.lastIndex;
  }

  // Handle any remaining text after the final escape sequence match
  const remainingText = escapedText.substring(lastIndex);
  if (remainingText) {
    const node = document.createElement('span');
    node.textContent = remainingText;
    applyStyle(node, currentStyle);
    elements.push(node);
  }

  return elements;
}

// applyStyle applies parsed color and formatting settings to a target HTML element
export function applyStyle(el: HTMLElement, styleObj: { color?: string; fontWeight?: string }): void {
  if (styleObj.color) el.style.color = styleObj.color;
  if (styleObj.fontWeight) el.style.fontWeight = styleObj.fontWeight;
}
