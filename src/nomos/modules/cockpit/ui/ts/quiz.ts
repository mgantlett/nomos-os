// quiz.ts - GitBrain Lore & Walkthrough Quiz Engine for Nomos OS

export interface QuizQuestion {
  id: string;
  question: string;
  options: string[];
  correctIndex: number;
  explanation: string;
}

export interface WalkthroughQuiz {
  taskId: string;
  title: string;
  questions: QuizQuestion[];
}

const sampleQuizzes: WalkthroughQuiz[] = [
  {
    taskId: '324',
    title: 'Task 324: Real-Time WebSocket Phase Status Push & Telemetry Live Stream',
    questions: [
      {
        id: 'q1',
        question: 'Why did WebSocket frame fragmentation occur in early browser clients?',
        options: [
          'The Go HTTP server dropped connections on port 8089',
          'Header and payload were sent in two separate conn.Write() TCP segments',
          'Client browsers required binary frames instead of text frames',
          'Substrate IPC lock blocked WebSocket writes'
        ],
        correctIndex: 1,
        explanation: 'Calling conn.Write(header) followed by conn.Write(payload) split the RFC 6455 frame across two distinct TCP segments, causing browser frame decoders to reject the payload.'
      },
      {
        id: 'q2',
        question: 'How did we eliminate the live log stream freeze when Substrate IPC logged events?',
        options: [
          'By polling nomos.jsonl every 100ms via HTTP REST',
          'By adding telemetry.GlobalBus.Publish() inside handleTelemetryStream in ipc.go',
          'By disabling LD_PRELOAD logging entirely',
          'By restarting nomosd after every git command'
        ],
        correctIndex: 1,
        explanation: 'Substrate IPC events were written directly to nomos.jsonl on disk, but were missing telemetry.GlobalBus.Publish(), preventing real-time WebSocket distribution.'
      }
    ]
  }
];

export function renderGitBrainQuizSection(target: string | HTMLElement, node?: any): void {
  const container = typeof target === 'string' ? document.getElementById(target) : target;
  if (!container) return;

  let quiz: WalkthroughQuiz = sampleQuizzes[0];
  if (node && node.insight) {
    try {
      const parsed = JSON.parse(node.insight);
      if (parsed && parsed.questions) {
        quiz = {
          taskId: parsed.taskId || node.commitHash || '326',
          title: parsed.title || 'Task Walkthrough Architectural Verification',
          questions: parsed.questions.map((q: any, idx: number) => ({
            id: `q${idx + 1}`,
            question: q.question,
            options: q.options || [],
            correctIndex: typeof q.correct_option === 'number' ? q.correct_option : (q.correctIndex || 0),
            explanation: q.explanation || 'Verified task architectural decision.'
          }))
        };
      }
    } catch (e) {}
  }

  let html = `
    <div style="background: rgba(139, 92, 246, 0.05); border: 1px solid var(--border-purple); border-radius: 8px; padding: 1.25rem; color: var(--text-main); height: 100%; overflow-y: auto;">
      <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem; padding-bottom: 0.75rem; border-bottom: 1px solid var(--border-indigo);">
        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 1.2rem;">🧠</span>
          <h3 style="margin: 0; font-size: 1rem; font-weight: bold; color: var(--neon-purple); font-family: 'Outfit', sans-serif;">GitBrain Architectural Quiz: ${quiz.title}</h3>
        </div>
        <span style="background: rgba(255, 184, 0, 0.15); color: var(--neon-yellow); font-size: 0.7rem; font-weight: bold; padding: 2px 8px; border-radius: 12px; font-family: 'JetBrains Mono', monospace;">+100 XP REWARD</span>
      </div>
  `;

  quiz.questions.forEach((q, qIdx) => {
    html += `
      <div style="background: var(--bg-glass); border: 1px solid var(--border-indigo); border-radius: 6px; padding: 1rem; margin-bottom: 0.75rem;">
        <div style="font-size: 0.85rem; font-weight: 600; margin-bottom: 0.75rem; color: var(--text-main);">Q${qIdx + 1}: ${q.question}</div>
        <div style="display: flex; flex-direction: column; gap: 6px;">
    `;

    q.options.forEach((opt, optIdx) => {
      html += `
        <button class="quiz-opt-btn" data-qid="${q.id}" data-opt="${optIdx}" data-correct="${q.correctIndex}" style="text-align: left; background: rgba(255, 255, 255, 0.03); border: 1px solid var(--border-indigo); color: var(--text-main); padding: 8px 12px; border-radius: 6px; cursor: pointer; font-size: 0.75rem; font-family: 'Outfit', sans-serif; transition: all 0.2s;">
          ${String.fromCharCode(65 + optIdx)}. ${opt}
        </button>
      `;
    });

    html += `
        </div>
        <div id="quiz-feedback-${q.id}" style="display: none; font-size: 0.75rem; margin-top: 8px; padding: 6px 10px; border-radius: 4px;"></div>
      </div>
    `;
  });

  html += `</div>`;
  container.innerHTML = html;

  // Bind quiz option click listeners
  container.querySelectorAll('.quiz-opt-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      const target = e.currentTarget as HTMLButtonElement;
      const qId = target.getAttribute('data-qid');
      const optIdx = parseInt(target.getAttribute('data-opt') || '0', 10);
      const correctIdx = parseInt(target.getAttribute('data-correct') || '0', 10);
      const feedbackEl = document.getElementById(`quiz-feedback-${qId}`);

      if (optIdx === correctIdx) {
        target.style.background = 'rgba(16, 185, 129, 0.2)';
        target.style.borderColor = 'var(--neon-green)';
        if (feedbackEl) {
          feedbackEl.style.display = 'block';
          feedbackEl.style.background = 'rgba(16, 185, 129, 0.15)';
          feedbackEl.style.color = 'var(--neon-green)';
          feedbackEl.innerHTML = `✅ Correct! Task architectural comprehension verified (+50 XP).`;
        }
      } else {
        target.style.background = 'rgba(239, 68, 68, 0.2)';
        target.style.borderColor = '#ef4444';
        if (feedbackEl) {
          feedbackEl.style.display = 'block';
          feedbackEl.style.background = 'rgba(239, 68, 68, 0.15)';
          feedbackEl.style.color = '#ef4444';
          feedbackEl.innerHTML = `❌ Incorrect. Review task walkthrough for architectural details.`;
        }
      }
    });
  });
}
