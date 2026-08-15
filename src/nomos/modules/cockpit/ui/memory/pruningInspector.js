import { renderMemoryClusterCanvas } from '../nebula.js';
import { cachedLessons, selectedLessonHash, activeTags, setSelectedLessonHash, setActiveTags, renderLessonsList, presetColors } from './memoryCore.js';
import { renderPruningAdvisorDashboard } from './pruningAdvisor.js';
import { saveMemoryEdit, pruneMemoryNode } from './pruningActions.js';
import { renderGitBrainQuizSection } from '../quiz.js';
export function selectMemoryNode(hash) {
    setSelectedLessonHash(hash);
    renderLessonsList();
    renderMemoryClusterCanvas(cachedLessons, selectedLessonHash, selectMemoryNode);
    const inspector = document.getElementById('memory-node-inspector');
    if (!inspector)
        return;
    if (!hash) {
        renderPruningAdvisorDashboard(inspector);
        return;
    }
    const node = cachedLessons.find(l => l.commitHash === hash);
    if (!node) {
        if (hash && hash.startsWith('quiz')) {
            renderGitBrainQuizSection(inspector);
            return;
        }
        renderPruningAdvisorDashboard(inspector);
        return;
    }
    if (node.category === 'quiz' || hash.startsWith('quiz')) {
        renderGitBrainQuizSection(inspector, node);
        return;
    }
    setActiveTags(Array.isArray(node.tags) ? [...node.tags] : []);
    let color = 'var(--neon-purple)';
    if (node.category && node.category !== 'general') {
        const uniqueCats = Array.from(new Set(cachedLessons.map(l => l.category).filter(Boolean))).sort();
        const idx = uniqueCats.indexOf(node.category);
        if (idx !== -1) {
            color = presetColors[idx % presetColors.length];
        }
    }
    else if (node.category === 'general') {
        color = 'var(--neon-purple)';
    }
    inspector.replaceChildren();
    inspector.style.textAlign = 'left';
    inspector.style.justifyContent = 'flex-start';
    inspector.style.height = '100%';
    inspector.style.overflow = 'hidden';
    const card = document.createElement('div');
    card.style.display = 'flex';
    card.style.flexDirection = 'column';
    card.style.height = '100%';
    card.style.width = '100%';
    card.style.justifyContent = 'space-between';
    card.style.overflow = 'hidden';
    const scrollBody = document.createElement('div');
    scrollBody.style.flex = '1';
    scrollBody.style.overflowY = 'auto';
    scrollBody.style.display = 'flex';
    scrollBody.style.flexDirection = 'column';
    scrollBody.style.gap = '12px';
    scrollBody.style.paddingRight = '4px';
    scrollBody.className = 'scrollable-indigo';
    const header = document.createElement('div');
    header.style.display = 'flex';
    header.style.justifyContent = 'space-between';
    header.style.alignItems = 'center';
    header.style.borderBottom = '1px solid rgba(255,255,255,0.05)';
    header.style.paddingBottom = '8px';
    const titleGroup = document.createElement('div');
    const hId = document.createElement('span');
    hId.style.fontFamily = "'JetBrains Mono', monospace";
    hId.style.fontWeight = 'bold';
    hId.style.fontSize = '0.95rem';
    hId.style.color = 'var(--text-main)';
    const charLen = node.insight ? node.insight.length : 0;
    hId.textContent = `${node.commitHash.substring(0, 7)} (${charLen} chars)`;
    const hTime = document.createElement('div');
    hTime.style.fontSize = '0.65rem';
    hTime.style.color = 'var(--text-muted)';
    hTime.textContent = new Date(node.timestamp).toLocaleString();
    titleGroup.appendChild(hId);
    titleGroup.appendChild(hTime);
    const catBadge = document.createElement('span');
    catBadge.style.fontSize = '0.65rem';
    catBadge.style.padding = '2px 8px';
    catBadge.style.borderRadius = '10px';
    catBadge.style.border = `1px solid ${color}`;
    catBadge.style.color = color;
    catBadge.style.fontWeight = 'bold';
    catBadge.textContent = node.category || 'general';
    const headerRight = document.createElement('div');
    headerRight.style.display = 'flex';
    headerRight.style.alignItems = 'center';
    headerRight.style.gap = '8px';
    headerRight.appendChild(catBadge);
    const closeBtn = document.createElement('span');
    closeBtn.textContent = '✕';
    closeBtn.style.cursor = 'pointer';
    closeBtn.style.fontSize = '0.85rem';
    closeBtn.style.color = 'var(--text-muted)';
    closeBtn.style.padding = '4px 8px';
    closeBtn.style.borderRadius = '4px';
    closeBtn.style.background = 'rgba(255,255,255,0.03)';
    closeBtn.style.transition = 'all 0.2s';
    closeBtn.onmouseover = () => {
        closeBtn.style.color = 'var(--text-main)';
        closeBtn.style.background = 'rgba(255,255,255,0.08)';
    };
    closeBtn.onmouseout = () => {
        closeBtn.style.color = 'var(--text-muted)';
        closeBtn.style.background = 'rgba(255,255,255,0.03)';
    };
    closeBtn.onclick = (e) => {
        e.stopPropagation();
        selectMemoryNode('');
    };
    headerRight.appendChild(closeBtn);
    header.appendChild(titleGroup);
    header.appendChild(headerRight);
    scrollBody.appendChild(header);
    const formGroup = document.createElement('div');
    formGroup.style.display = 'flex';
    formGroup.style.flexDirection = 'column';
    formGroup.style.gap = '8px';
    const labelCat = document.createElement('label');
    labelCat.style.fontSize = '0.65rem';
    labelCat.style.color = 'var(--text-muted)';
    labelCat.style.textTransform = 'uppercase';
    labelCat.textContent = 'Category';
    const selectCat = document.createElement('select');
    selectCat.id = 'edit-memory-category';
    selectCat.style.background = 'rgba(var(--bg-dark-rgb), 0.9)';
    selectCat.style.border = '1px solid var(--border-indigo)';
    selectCat.style.color = 'var(--text-main)';
    selectCat.style.fontFamily = "'Outfit', sans-serif";
    selectCat.style.fontSize = '0.8rem';
    selectCat.style.padding = '6px';
    selectCat.style.borderRadius = '4px';
    const uniqueCats = new Set(['metadata', 'submodules', 'xss-security', 'tdd-gates', 'general', 'walkthrough', 'explainer', 'quiz']);
    cachedLessons.forEach(l => {
        if (l.category && l.category.trim())
            uniqueCats.add(l.category.trim());
    });
    const categoriesList = Array.from(uniqueCats).sort();
    categoriesList.forEach(cat => {
        const opt = document.createElement('option');
        opt.value = cat;
        opt.textContent = cat;
        opt.selected = (node.category || 'general') === cat;
        selectCat.appendChild(opt);
    });
    const labelTags = document.createElement('label');
    labelTags.style.fontSize = '0.65rem';
    labelTags.style.color = 'var(--text-muted)';
    labelTags.style.textTransform = 'uppercase';
    labelTags.textContent = 'Tags';
    const tagsWrapper = document.createElement('div');
    tagsWrapper.id = 'edit-memory-tags-list';
    tagsWrapper.style.display = 'flex';
    tagsWrapper.style.flexWrap = 'wrap';
    tagsWrapper.style.gap = '6px';
    tagsWrapper.style.marginBottom = '4px';
    const renderEditTags = () => {
        tagsWrapper.replaceChildren();
        activeTags.forEach((tg, tgIdx) => {
            const badg = document.createElement('span');
            badg.style.fontSize = '0.7rem';
            badg.style.padding = '2px 8px';
            badg.style.borderRadius = '12px';
            badg.style.background = 'rgba(255, 255, 255, 0.08)';
            badg.style.color = 'var(--text-main)';
            badg.style.display = 'inline-flex';
            badg.style.alignItems = 'center';
            badg.style.gap = '4px';
            badg.textContent = tg;
            const delBtn = document.createElement('span');
            delBtn.textContent = '×';
            delBtn.style.cursor = 'pointer';
            delBtn.style.color = 'var(--neon-pink)';
            delBtn.style.fontWeight = 'bold';
            delBtn.onclick = () => {
                activeTags.splice(tgIdx, 1);
                renderEditTags();
            };
            badg.appendChild(delBtn);
            tagsWrapper.appendChild(badg);
        });
    };
    renderEditTags();
    const inputRow = document.createElement('div');
    inputRow.style.display = 'flex';
    inputRow.style.gap = '6px';
    inputRow.style.marginBottom = '6px';
    const tagInput = document.createElement('input');
    tagInput.type = 'text';
    tagInput.placeholder = 'Add tag...';
    tagInput.style.flex = '1';
    tagInput.style.background = 'rgba(var(--bg-dark-rgb), 0.9)';
    tagInput.style.border = '1px solid var(--border-indigo)';
    tagInput.style.color = 'var(--text-main)';
    tagInput.style.fontFamily = "'Outfit', sans-serif";
    tagInput.style.fontSize = '0.8rem';
    tagInput.style.padding = '4px 8px';
    tagInput.style.borderRadius = '4px';
    const addBtn = document.createElement('button');
    addBtn.textContent = '+';
    addBtn.style.background = 'var(--neon-purple)';
    addBtn.style.color = 'var(--text-main)';
    addBtn.style.border = 'none';
    addBtn.style.padding = '4px 10px';
    addBtn.style.borderRadius = '4px';
    addBtn.style.cursor = 'pointer';
    addBtn.style.fontWeight = 'bold';
    const addTagFn = () => {
        const val = tagInput.value.trim();
        if (val && !activeTags.includes(val)) {
            activeTags.push(val);
            renderEditTags();
            tagInput.value = '';
        }
    };
    addBtn.onclick = (e) => {
        e.preventDefault();
        addTagFn();
    };
    tagInput.onkeydown = (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            addTagFn();
        }
    };
    inputRow.appendChild(tagInput);
    inputRow.appendChild(addBtn);
    const labelInsight = document.createElement('label');
    labelInsight.style.fontSize = '0.65rem';
    labelInsight.style.color = 'var(--text-muted)';
    labelInsight.style.textTransform = 'uppercase';
    labelInsight.textContent = 'Insight Description';
    const contentWrapper = document.createElement('div');
    contentWrapper.style.display = 'flex';
    contentWrapper.style.flexDirection = 'column';
    contentWrapper.style.gap = '8px';
    contentWrapper.style.flex = '1';
    if (node.category !== 'quiz') {
        const mdView = document.createElement('div');
        mdView.className = 'markdown-body';
        mdView.style.background = 'rgba(var(--bg-dark-rgb), 0.9)';
        mdView.style.border = '1px solid var(--border-indigo)';
        mdView.style.padding = '12px';
        mdView.style.borderRadius = '4px';
        mdView.style.color = 'var(--text-main)';
        mdView.style.fontSize = '0.85rem';
        mdView.style.overflowY = 'auto';
        mdView.style.flex = '1';
        // Check if marked is available
        if (window.marked) {
            mdView.innerHTML = window.marked.parse(node.insight || '');
        }
        else {
            mdView.textContent = node.insight;
        }
        contentWrapper.appendChild(mdView);
        // Keep hidden textarea for saving edits
        const hiddenTextarea = document.createElement('textarea');
        hiddenTextarea.id = 'edit-memory-insight';
        hiddenTextarea.style.display = 'none';
        hiddenTextarea.value = node.insight;
        contentWrapper.appendChild(hiddenTextarea);
    }
    else {
        const quizView = document.createElement('div');
        quizView.style.background = 'rgba(var(--bg-dark-rgb), 0.9)';
        quizView.style.border = '1px solid var(--border-indigo)';
        quizView.style.padding = '12px';
        quizView.style.borderRadius = '4px';
        quizView.style.color = 'var(--text-main)';
        quizView.style.fontSize = '0.85rem';
        quizView.style.overflowY = 'auto';
        quizView.style.flex = '1';
        quizView.style.display = 'flex';
        quizView.style.flexDirection = 'column';
        quizView.style.gap = '16px';
        try {
            const quizData = JSON.parse(node.insight);
            const title = document.createElement('h3');
            title.textContent = quizData.title || 'Quiz';
            title.style.margin = '0 0 8px 0';
            title.style.color = 'var(--neon-blue)';
            quizView.appendChild(title);
            if (Array.isArray(quizData.questions)) {
                quizData.questions.forEach((q, i) => {
                    const qBlock = document.createElement('div');
                    qBlock.style.border = '1px solid rgba(255,255,255,0.1)';
                    qBlock.style.padding = '10px';
                    qBlock.style.borderRadius = '6px';
                    const qText = document.createElement('div');
                    qText.style.fontWeight = 'bold';
                    qText.style.marginBottom = '8px';
                    qText.textContent = `${i + 1}. ${q.question}`;
                    qBlock.appendChild(qText);
                    if (Array.isArray(q.options)) {
                        q.options.forEach((opt, optIdx) => {
                            const optBtn = document.createElement('button');
                            optBtn.style.display = 'block';
                            optBtn.style.width = '100%';
                            optBtn.style.textAlign = 'left';
                            optBtn.style.padding = '6px 10px';
                            optBtn.style.marginBottom = '4px';
                            optBtn.style.background = 'rgba(255,255,255,0.05)';
                            optBtn.style.border = '1px solid rgba(255,255,255,0.1)';
                            optBtn.style.color = '#ccc';
                            optBtn.style.borderRadius = '4px';
                            optBtn.style.cursor = 'pointer';
                            optBtn.textContent = opt;
                            optBtn.onclick = () => {
                                if (optIdx === q.correct_option) {
                                    optBtn.style.background = 'rgba(0, 255, 0, 0.2)';
                                    optBtn.style.borderColor = 'var(--neon-green)';
                                    optBtn.style.color = 'var(--text-main)';
                                    if (q.explanation) {
                                        const expl = document.createElement('div');
                                        expl.style.marginTop = '8px';
                                        expl.style.color = 'var(--neon-green)';
                                        expl.style.fontSize = '0.75rem';
                                        expl.textContent = `Correct! ${q.explanation}`;
                                        qBlock.appendChild(expl);
                                    }
                                }
                                else {
                                    optBtn.style.background = 'rgba(255, 0, 0, 0.2)';
                                    optBtn.style.borderColor = 'var(--neon-pink)';
                                }
                            };
                            qBlock.appendChild(optBtn);
                        });
                    }
                    quizView.appendChild(qBlock);
                });
            }
        }
        catch (e) {
            quizView.textContent = 'Invalid quiz data.';
        }
        contentWrapper.appendChild(quizView);
        const hiddenTextarea = document.createElement('textarea');
        hiddenTextarea.id = 'edit-memory-insight';
        hiddenTextarea.style.display = 'none';
        hiddenTextarea.value = node.insight;
        contentWrapper.appendChild(hiddenTextarea);
    }
    formGroup.appendChild(labelCat);
    formGroup.appendChild(selectCat);
    formGroup.appendChild(labelTags);
    formGroup.appendChild(tagsWrapper);
    formGroup.appendChild(inputRow);
    formGroup.appendChild(labelInsight);
    formGroup.appendChild(contentWrapper);
    scrollBody.appendChild(formGroup);
    card.appendChild(scrollBody);
    const actions = document.createElement('div');
    actions.style.display = 'flex';
    actions.style.gap = '8px';
    actions.style.marginTop = '12px';
    actions.style.paddingTop = '8px';
    actions.style.borderTop = '1px solid rgba(255,255,255,0.05)';
    const saveBtn = document.createElement('button');
    saveBtn.style.flex = '1';
    saveBtn.style.background = 'var(--neon-purple)';
    saveBtn.style.color = 'var(--text-main)';
    saveBtn.style.border = 'none';
    saveBtn.style.padding = '6px 12px';
    saveBtn.style.borderRadius = '4px';
    saveBtn.style.fontSize = '0.75rem';
    saveBtn.style.fontWeight = 'bold';
    saveBtn.style.cursor = 'pointer';
    saveBtn.textContent = '💾 SAVE CHANGES';
    saveBtn.onclick = () => saveMemoryEdit(node.commitHash);
    const pruneBtn = document.createElement('button');
    pruneBtn.style.background = 'transparent';
    pruneBtn.style.color = 'var(--neon-pink)';
    pruneBtn.style.border = '1px solid var(--neon-pink)';
    pruneBtn.style.padding = '6px 12px';
    pruneBtn.style.borderRadius = '4px';
    pruneBtn.style.fontSize = '0.75rem';
    pruneBtn.style.fontWeight = 'bold';
    pruneBtn.style.cursor = 'pointer';
    pruneBtn.textContent = '✂️ PRUNE';
    pruneBtn.onclick = () => pruneMemoryNode(node.commitHash);
    actions.appendChild(saveBtn);
    actions.appendChild(pruneBtn);
    card.appendChild(actions);
    inspector.appendChild(card);
}
