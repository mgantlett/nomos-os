// splitters.ts - Vertical & horizontal layout splitters handler
// This module implements click-and-drag handles (splitters) to resize the sidebar and the Kanban board
// panel in the Cockpit UI dashboard, including localstorage caching for layouts.

// SplitterDrag handles resizing the bottom Kanban panel in the main console layout.
class SplitterDrag {
  private startY = 0; // Vertical cursor starting coordinate
  private startHeight = 0; // Panel height offset before drag operation begins
  private kanbanPanel: HTMLElement; // Reference to target Kanban board DOM container
  private splitter: HTMLElement; // resizable splitter handle element
  private storageKey: string; // Key to use for localstorage persistence

  constructor(splitter: HTMLElement, kanbanPanel: HTMLElement, storageKey = 'nomos-cockpit-splitter-height') {
    this.splitter = splitter;
    this.kanbanPanel = kanbanPanel;
    this.storageKey = storageKey;
    // Bind mouse down listener to initial drag trigger
    this.splitter.addEventListener('mousedown', (e) => this.onMouseDown(e));
  }

  // Handle click on resizer handle and attach global listeners
  private onMouseDown(e: MouseEvent) {
    e.preventDefault();
    this.splitter.classList.add('dragging');
    document.body.classList.add('dragging-active');
    document.body.style.cursor = 'ns-resize'; // Style body cursor for vertical resize indicator
    this.startY = e.clientY;
    this.startHeight = this.kanbanPanel.offsetHeight;

    // Bind function context for cleanup listeners
    const onMove = (moveEvent: MouseEvent) => this.onMouseMove(moveEvent);
    const onUp = () => this.onMouseUp(onMove, onUp);

    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  // Handle active cursor movement and calculate delta height changes
  private onMouseMove(moveEvent: MouseEvent) {
    const deltaY = moveEvent.clientY - this.startY;
    let newHeight = this.startHeight + deltaY;
    // Enforce viewport boundary constraints (between 20% and 80% inner window height)
    const minHeight = window.innerHeight * 0.20;
    const maxHeight = window.innerHeight * 0.80;
    if (newHeight < minHeight) newHeight = minHeight;
    if (newHeight > maxHeight) newHeight = maxHeight;
    this.kanbanPanel.style.height = newHeight + 'px';
  }

  // Clean up window level listeners and cache height persistently
  private onMouseUp(onMove: any, onUp: any) {
    this.splitter.classList.remove('dragging');
    document.body.classList.remove('dragging-active');
    document.body.style.cursor = '';
    // Store height to survive browser reloads
    localStorage.setItem(this.storageKey, this.kanbanPanel.style.height);
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mouseup', onUp);
  }
}

// SidebarDrag handles layout splitters to resize the navigation aside drawer.
class SidebarDrag {
  private startX = 0; // Horizontal cursor starting coordinate
  private startWidth = 0; // Sidebar width before drag operation begins
  private cockpitContainer: HTMLElement; // Root container element
  private splitter: HTMLElement; // Resizer sidebar splitter element

  constructor(splitter: HTMLElement, cockpitContainer: HTMLElement) {
    this.splitter = splitter;
    this.cockpitContainer = cockpitContainer;
    this.splitter.addEventListener('mousedown', (e) => this.onMouseDown(e));
  }

  // Capture start layout coordinates and bind drag listeners
  private onMouseDown(e: MouseEvent) {
    e.preventDefault();
    this.splitter.classList.add('dragging');
    document.body.classList.add('dragging-active');
    document.body.style.cursor = 'ew-resize'; // Style body cursor for horizontal resize indicator
    const asideElement = document.querySelector('aside') as HTMLElement | null;
    this.startWidth = asideElement ? asideElement.offsetWidth : 320;
    this.startX = e.clientX;

    // Keep context reference during window drag movement tracking
    const onMove = (moveEvent: MouseEvent) => this.onMouseMove(moveEvent);
    const onUp = () => this.onMouseUp(onMove, onUp);

    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  // Perform active resizing of the aside drawer
  private onMouseMove(moveEvent: MouseEvent) {
    const deltaX = moveEvent.clientX - this.startX;
    let newWidth = this.startWidth + deltaX;
    // Clamp values between 240px and 480px width limits
    if (newWidth < 240) newWidth = 240;
    if (newWidth > 480) newWidth = 480;
    this.cockpitContainer.style.setProperty('--sidebar-width', newWidth + 'px');
  }

  // Prune movement handlers and save width to local storage
  private onMouseUp(onMove: any, onUp: any) {
    this.splitter.classList.remove('dragging');
    document.body.classList.remove('dragging-active');
    document.body.style.cursor = '';
    const pxWidth = this.cockpitContainer.style.getPropertyValue('--sidebar-width');
    localStorage.setItem('nomos-cockpit-sidebar-width', pxWidth);
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mouseup', onUp);
  }
}

// Initializer function mapped to UI DOM load triggers.
export function initSplitters(): void {
  const splitter = document.getElementById('cockpit-splitter');
  const kanbanPanel = (document.querySelector('.kanban-panel') || document.getElementById('active-swarm-hud')) as HTMLElement | null;
  const sidebarSplitter = document.getElementById('sidebar-splitter');
  const cockpitContainer = document.querySelector('.cockpit-container') as HTMLElement | null;

  // Restore cached splitter height
  const persistedHeight = localStorage.getItem('nomos-cockpit-splitter-height');
  if (persistedHeight && kanbanPanel) {
    let parsedHeight = parseInt(persistedHeight, 10);
    if (!isNaN(parsedHeight)) {
      const minHeight = window.innerHeight * 0.20;
      const maxHeight = window.innerHeight * 0.80;
      if (parsedHeight < minHeight) parsedHeight = minHeight;
      if (parsedHeight > maxHeight) parsedHeight = maxHeight;
      kanbanPanel.style.height = parsedHeight + 'px';
    }
  }

  // Instantiate drag handlers for vertical splitter
  if (splitter && kanbanPanel) {
    new SplitterDrag(splitter, kanbanPanel, 'nomos-cockpit-splitter-height');
  }

  // Restore cached ultimate splitter height
  const ultimateSplitter = document.getElementById('ultimate-horizontal-splitter');
  const ultimateTopHalf = document.getElementById('ultimate-top-half');
  const persistedUltimateHeight = localStorage.getItem('nomos-ultimate-splitter-height');
  if (persistedUltimateHeight && ultimateTopHalf) {
    let parsedHeight = parseInt(persistedUltimateHeight, 10);
    if (!isNaN(parsedHeight)) {
      const minHeight = window.innerHeight * 0.20;
      const maxHeight = window.innerHeight * 0.80;
      if (parsedHeight < minHeight) parsedHeight = minHeight;
      if (parsedHeight > maxHeight) parsedHeight = maxHeight;
      ultimateTopHalf.style.height = parsedHeight + 'px';
    }
  }

  // Instantiate drag handler for ultimate vertical splitter
  if (ultimateSplitter && ultimateTopHalf) {
    new SplitterDrag(ultimateSplitter, ultimateTopHalf, 'nomos-ultimate-splitter-height');
  }

  // Restore cached aside drawer width
  const persistedWidth = localStorage.getItem('nomos-cockpit-sidebar-width');
  if (persistedWidth && cockpitContainer) {
    let parsedWidth = parseInt(persistedWidth, 10);
    if (!isNaN(parsedWidth)) {
      if (parsedWidth < 240) parsedWidth = 240;
      if (parsedWidth > 480) parsedWidth = 480;
      cockpitContainer.style.setProperty('--sidebar-width', parsedWidth + 'px');
    }
  }

  // Instantiate drag handlers for aside splitter
  if (sidebarSplitter && cockpitContainer) {
    new SidebarDrag(sidebarSplitter, cockpitContainer);
  }

  // Restore cached details column split ratio
  const detailsSplitter = document.getElementById('details-column-splitter');
  const detailsLeftPane = document.getElementById('details-left-pane');
  const persistedDetailsRatio = localStorage.getItem('nomos-details-split-ratio');
  if (persistedDetailsRatio && detailsLeftPane) {
    let parsedRatio = parseFloat(persistedDetailsRatio);
    if (!isNaN(parsedRatio)) {
      if (parsedRatio < 20) parsedRatio = 20;
      if (parsedRatio > 80) parsedRatio = 80;
      detailsLeftPane.style.flex = `0 0 ${parsedRatio}%`;
    }
  }
  if (detailsSplitter && detailsLeftPane) {
    new ColumnSplitterDrag(detailsSplitter, detailsLeftPane);
  }
}

// ColumnSplitterDrag handles horizontal resizing between Task Details (left) and Agent Details (right)
class ColumnSplitterDrag {
  private startX = 0;
  private startLeftWidth = 0;
  private leftPane: HTMLElement;
  private splitter: HTMLElement;

  constructor(splitter: HTMLElement, leftPane: HTMLElement) {
    this.splitter = splitter;
    this.leftPane = leftPane;
    this.splitter.addEventListener('mousedown', (e) => this.onMouseDown(e));
  }

  private onMouseDown(e: MouseEvent) {
    e.preventDefault();
    this.splitter.classList.add('dragging');
    document.body.classList.add('dragging-active');
    document.body.style.cursor = 'col-resize';
    this.startX = e.clientX;
    this.startLeftWidth = this.leftPane.offsetWidth;

    const onMove = (moveEvent: MouseEvent) => this.onMouseMove(moveEvent);
    const onUp = () => this.onMouseUp(onMove, onUp);

    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  private onMouseMove(moveEvent: MouseEvent) {
    const parent = this.leftPane.parentElement;
    if (!parent) return;
    const totalWidth = parent.offsetWidth;
    const deltaX = moveEvent.clientX - this.startX;
    let newWidth = this.startLeftWidth + deltaX;
    let percentage = (newWidth / totalWidth) * 100;
    if (percentage < 20) percentage = 20;
    if (percentage > 80) percentage = 80;
    this.leftPane.style.flex = `0 0 ${percentage}%`;
  }

  private onMouseUp(onMove: any, onUp: any) {
    this.splitter.classList.remove('dragging');
    document.body.classList.remove('dragging-active');
    document.body.style.cursor = '';
    const parent = this.leftPane.parentElement;
    if (parent) {
      const percentage = (this.leftPane.offsetWidth / parent.offsetWidth) * 100;
      localStorage.setItem('nomos-details-split-ratio', percentage.toFixed(1));
    }
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mouseup', onUp);
  }
}

