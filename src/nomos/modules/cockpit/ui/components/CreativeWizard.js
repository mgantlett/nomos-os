export class CreativeWizard {
    constructor() {
        this.overlay = document.getElementById('creative-wizard-overlay');
        this.closeBtn = document.getElementById('creative-wizard-close');
        this.launchBtn = document.getElementById('creative-wizard-btn');
        
        this.step1 = document.getElementById('cw-step-1');
        this.step2 = document.getElementById('cw-step-2');
        this.step3 = document.getElementById('cw-step-3');
        this.loading = document.getElementById('cw-loading');
        
        this.visionInput = document.getElementById('cw-vision-input');
        this.discoveryForm = document.getElementById('cw-discovery-form');
        this.dagPreview = document.getElementById('cw-dag-preview');
        
        this.btnDiscover = document.getElementById('cw-btn-discover');
        this.btnArchitect = document.getElementById('cw-btn-architect');
        this.btnCommit = document.getElementById('cw-btn-commit');
        this.btnBack1 = document.getElementById('cw-btn-back-1');
        this.btnBack2 = document.getElementById('cw-btn-back-2');
        
        this.vision = '';
        this.dagPayload = null;

        if (this.overlay) {
            this.bindEvents();
        }
    }

    bindEvents() {
        if(this.launchBtn) {
            this.launchBtn.addEventListener('click', () => this.open());
        }
        if(this.closeBtn) {
            this.closeBtn.addEventListener('click', () => this.close());
        }
        
        this.btnDiscover.addEventListener('click', () => this.runDiscovery());
        this.btnArchitect.addEventListener('click', () => this.runArchitecture());
        this.btnCommit.addEventListener('click', () => this.commitDAG());
        
        this.btnBack1.addEventListener('click', () => this.showStep(1));
        this.btnBack2.addEventListener('click', () => this.showStep(2));
    }

    open() {
        this.overlay.style.display = 'flex';
        this.showStep(1);
    }

    close() {
        this.overlay.style.display = 'none';
        this.visionInput.value = '';
        this.discoveryForm.innerHTML = '';
        this.dagPreview.innerHTML = '';
        this.vision = '';
        this.dagPayload = null;
    }

    showStep(stepNum) {
        this.step1.style.display = stepNum === 1 ? 'block' : 'none';
        this.step2.style.display = stepNum === 2 ? 'block' : 'none';
        this.step3.style.display = stepNum === 3 ? 'block' : 'none';
    }

    setLoading(isLoading) {
        this.loading.style.display = isLoading ? 'flex' : 'none';
    }

    async runDiscovery() {
        this.vision = this.visionInput.value.trim();
        if(!this.vision) {
            alert('Please provide a vision first.');
            return;
        }

        this.setLoading(true);
        try {
            const res = await fetch('/api/creative/discovery', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ vision: this.vision })
            });
            if (!res.ok) throw new Error(await res.text());
            
            const data = await res.json();
            
            this.discoveryForm.innerHTML = '';
            (data.questions || []).forEach((q, idx) => {
                this.discoveryForm.innerHTML += `
                    <div style="background: rgba(255,255,255,0.02); padding: 12px; border-radius: 6px; border: 1px solid #30363d;">
                        <label style="display: block; color: #00f0ff; font-size: 0.85rem; font-weight: bold; margin-bottom: 8px;">Q${idx+1}: ${q}</label>
                        <textarea class="cw-discovery-answer" data-question="${q.replace(/"/g, '&quot;')}" rows="2" style="width: 100%; box-sizing: border-box; background: #0d1117; border: 1px solid #30363d; border-radius: 4px; padding: 8px; color: #c9d1d9; resize: vertical;" placeholder="Your answer..."></textarea>
                    </div>
                `;
            });
            this.showStep(2);
        } catch (err) {
            alert('Discovery failed: ' + err.message);
        } finally {
            this.setLoading(false);
        }
    }

    async runArchitecture() {
        let answersArr = [];
        const textareas = this.discoveryForm.querySelectorAll('.cw-discovery-answer');
        textareas.forEach(ta => {
            const q = ta.getAttribute('data-question');
            const a = ta.value.trim();
            if(a) {
                answersArr.push(`Q: ${q}\nA: ${a}`);
            }
        });
        const answers = answersArr.join('\n\n');

        this.setLoading(true);
        try {
            const res = await fetch('/api/creative/architect', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ vision: this.vision, answers: answers })
            });
            if (!res.ok) throw new Error(await res.text());
            
            this.dagPayload = await res.json();
            
            this.dagPreview.innerHTML = `<strong>Blueprint:</strong>\n${this.dagPayload.blueprint}\n\n<strong>Tasks:</strong>\n${JSON.stringify(this.dagPayload.tasks, null, 2)}`;
            this.showStep(3);
        } catch (err) {
            alert('Architecture generation failed: ' + err.message);
        } finally {
            this.setLoading(false);
        }
    }

    async commitDAG() {
        if(!this.dagPayload) return;
        this.setLoading(true);
        try {
            const res = await fetch('/api/creative/commit', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ 
                    vision: this.vision, 
                    blueprint: this.dagPayload.blueprint,
                    tasks: this.dagPayload.tasks
                })
            });
            if(res.ok) {
                alert('DAG Successfully committed to SQLite tracker!');
                this.close();
                if(window.refreshData) window.refreshData(); // If there is a global refresh
            } else {
                throw new Error(await res.text());
            }
        } catch (err) {
            alert('Commit failed: ' + err.message);
        } finally {
            this.setLoading(false);
        }
    }
}
