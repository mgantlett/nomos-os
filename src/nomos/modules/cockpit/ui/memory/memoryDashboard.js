import { openTaskDetailsDrawer } from '../modal.js';
let cachedDebtData = null;
let lastDebtFetchTime = 0;
export function renderTechnicalDebtRegistry(listContainer) {
    listContainer.replaceChildren();
    const loadingMsg = document.createElement('div');
    loadingMsg.style.fontSize = '0.65rem';
    loadingMsg.style.color = 'rgba(255,255,255,0.4)';
    loadingMsg.style.textAlign = 'center';
    loadingMsg.style.padding = '8px';
    loadingMsg.textContent = 'Loading registry...';
    listContainer.appendChild(loadingMsg);
    const renderData = (data) => {
        listContainer.replaceChildren();
        if (!data.success || !data.active_debt || data.active_debt.length === 0) {
            const emptyMsg = document.createElement('div');
            emptyMsg.style.fontSize = '0.65rem';
            emptyMsg.style.color = 'var(--neon-green)';
            emptyMsg.style.textAlign = 'center';
            emptyMsg.style.padding = '8px';
            emptyMsg.style.background = 'rgba(16, 185, 129, 0.05)';
            emptyMsg.style.borderRadius = '4px';
            emptyMsg.style.border = '1px solid rgba(16, 185, 129, 0.15)';
            emptyMsg.textContent = 'No active technical debt or quality gate bypasses.';
            listContainer.appendChild(emptyMsg);
            return;
        }
        data.active_debt.forEach(item => {
            const itemRow = document.createElement('div');
            itemRow.style.display = 'flex';
            itemRow.style.flexDirection = 'column';
            itemRow.style.gap = '4px';
            itemRow.style.padding = '8px';
            itemRow.style.background = 'rgba(255, 255, 255, 0.01)';
            itemRow.style.border = '1px solid rgba(255, 255, 255, 0.03)';
            itemRow.style.borderRadius = '6px';
            itemRow.style.transition = 'border-color 0.2s ease';
            const rowTop = document.createElement('div');
            rowTop.style.display = 'flex';
            rowTop.style.justifyContent = 'space-between';
            rowTop.style.alignItems = 'center';
            const fileEl = document.createElement('div');
            fileEl.style.fontSize = '0.7rem';
            fileEl.style.fontWeight = 'bold';
            fileEl.style.color = 'var(--text-main)';
            fileEl.style.fontFamily = 'monospace';
            fileEl.style.whiteSpace = 'nowrap';
            fileEl.style.overflow = 'hidden';
            fileEl.style.textOverflow = 'ellipsis';
            fileEl.style.maxWidth = '65%';
            fileEl.textContent = item.file;
            fileEl.title = item.file;
            const badgeContainer = document.createElement('div');
            badgeContainer.style.display = 'flex';
            badgeContainer.style.gap = '4px';
            // Task badge
            const taskBadge = document.createElement('span');
            taskBadge.style.fontSize = '0.6rem';
            taskBadge.style.fontWeight = 'bold';
            taskBadge.style.padding = '1px 6px';
            taskBadge.style.borderRadius = '4px';
            taskBadge.style.cursor = 'pointer';
            taskBadge.style.background = 'rgba(147, 51, 234, 0.15)';
            taskBadge.style.border = '1px solid rgba(147, 51, 234, 0.3)';
            taskBadge.style.color = '#c084fc';
            taskBadge.textContent = `#${item.linked_task}`;
            taskBadge.title = 'Click to open details drawer';
            taskBadge.onclick = (e) => {
                e.stopPropagation();
                const tId = parseInt(item.linked_task, 10);
                if (!isNaN(tId)) {
                    openTaskDetailsDrawer(tId);
                }
            };
            // Expiration math
            const expiryDate = new Date(item.expires_at);
            const now = new Date();
            const isExpired = isNaN(expiryDate.getTime()) || now > expiryDate;
            const msPerDay = 24 * 60 * 60 * 1000;
            const daysLeft = (expiryDate.getTime() - now.getTime()) / msPerDay;
            const isExpiringSoon = !isExpired && daysLeft > 0 && daysLeft <= 3;
            const isClosed = !item.isOpen;
            // Status badge
            const statusBadge = document.createElement('span');
            statusBadge.style.fontSize = '0.6rem';
            statusBadge.style.fontWeight = 'bold';
            statusBadge.style.padding = '1px 6px';
            statusBadge.style.borderRadius = '4px';
            if (isExpired) {
                statusBadge.style.background = 'rgba(239, 68, 68, 0.15)';
                statusBadge.style.border = '1px solid rgba(239, 68, 68, 0.3)';
                statusBadge.style.color = '#ef4444';
                statusBadge.textContent = 'EXPIRED';
                itemRow.style.borderColor = 'rgba(239, 68, 68, 0.2)';
            }
            else if (isClosed) {
                statusBadge.style.background = 'rgba(239, 68, 68, 0.15)';
                statusBadge.style.border = '1px solid rgba(239, 68, 68, 0.3)';
                statusBadge.style.color = '#ef4444';
                statusBadge.textContent = 'DONE';
                itemRow.style.borderColor = 'rgba(239, 68, 68, 0.2)';
            }
            else if (isExpiringSoon) {
                statusBadge.style.background = 'rgba(245, 158, 11, 0.15)';
                statusBadge.style.border = '1px solid rgba(245, 158, 11, 0.3)';
                statusBadge.style.color = '#f59e0b';
                statusBadge.textContent = `${Math.ceil(daysLeft)}d left`;
                itemRow.style.borderColor = 'rgba(245, 158, 11, 0.2)';
            }
            else {
                statusBadge.style.background = 'rgba(16, 185, 129, 0.15)';
                statusBadge.style.border = '1px solid rgba(16, 185, 129, 0.3)';
                statusBadge.style.color = '#10b981';
                statusBadge.textContent = 'ACTIVE';
                itemRow.style.borderColor = 'rgba(16, 185, 129, 0.2)';
            }
            badgeContainer.appendChild(taskBadge);
            badgeContainer.appendChild(statusBadge);
            rowTop.appendChild(fileEl);
            rowTop.appendChild(badgeContainer);
            itemRow.appendChild(rowTop);
            // Details row
            const detailsRow = document.createElement('div');
            detailsRow.style.display = 'flex';
            detailsRow.style.justifyContent = 'space-between';
            detailsRow.style.fontSize = '0.62rem';
            detailsRow.style.color = 'var(--text-muted)';
            detailsRow.style.marginTop = '2px';
            const gateInfo = document.createElement('span');
            gateInfo.style.fontFamily = 'monospace';
            gateInfo.style.color = 'rgba(255,255,255,0.6)';
            gateInfo.textContent = `Gate: ${item.gate}`;
            const expiryInfo = document.createElement('span');
            expiryInfo.textContent = `Expires: ${item.expires_at}`;
            detailsRow.appendChild(gateInfo);
            detailsRow.appendChild(expiryInfo);
            itemRow.appendChild(detailsRow);
            if (item.reason) {
                const reasonEl = document.createElement('div');
                reasonEl.style.fontSize = '0.62rem';
                reasonEl.style.color = 'rgba(255,255,255,0.45)';
                reasonEl.style.fontStyle = 'italic';
                reasonEl.style.marginTop = '2px';
                reasonEl.textContent = `Reason: ${item.reason}`;
                itemRow.appendChild(reasonEl);
            }
            listContainer.appendChild(itemRow);
        });
    };
    if (cachedDebtData) {
        // We already have cached data. Let's just render it if we haven't already.
        // If the first child is NOT the loading message, we already rendered it!
        if (listContainer.firstElementChild && listContainer.firstElementChild.textContent !== 'Loading registry...') {
            return;
        }
        renderData(cachedDebtData);
        return;
    }
    fetch('/api/quality-debt')
        .then(res => {
        if (!res.ok)
            throw new Error('Failed to fetch');
        return res.json();
    })
        .then(data => {
        cachedDebtData = data;
        renderData(data);
    })
        .catch(() => {
        listContainer.replaceChildren();
        const errorMsg = document.createElement('div');
        errorMsg.style.fontSize = '0.65rem';
        errorMsg.style.color = 'var(--neon-pink)';
        errorMsg.style.textAlign = 'center';
        errorMsg.style.padding = '8px';
        errorMsg.textContent = 'Failed to load debt registry.';
        listContainer.appendChild(errorMsg);
    });
}
