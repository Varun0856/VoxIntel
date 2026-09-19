const FIXED_PROMPT = "Generate the Top Takeaways for this session.";

const els = {
  startBtn: document.getElementById('startBtn'),
  stopBtn: document.getElementById('stopBtn'),
  micSelect: document.getElementById('micSelect'),
  recDot: document.getElementById('recDot'),
  recStatus: document.getElementById('recStatus'),
  sessionPanel: document.getElementById('sessionPanel'),
  sessionStatus: document.getElementById('sessionStatus'),
  transcript: document.getElementById('transcript'),
  summaryBtn: document.getElementById('summaryBtn'),
  exportBtn: document.getElementById('exportBtn'),
  deleteBtn: document.getElementById('deleteBtn'),
  summaryBox: document.getElementById('summaryBox'),
  summaryText: document.getElementById('summaryText'),
  sessionList: document.getElementById('sessionList'),
  historyEmpty: document.getElementById('historyEmpty'),
  errorBanner: document.getElementById('errorBanner'),
  themeToggle: document.getElementById('themeToggle'),
  iconMoon: document.getElementById('iconMoon'),
  iconSun: document.getElementById('iconSun'),
};

let mediaRecorder = null;
let recordedChunks = [];
let currentSessionId = null;
let pollHandle = null;

// --- Theme ---

function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  els.iconMoon.classList.toggle('hidden', theme === 'light');
  els.iconSun.classList.toggle('hidden', theme === 'dark');
  localStorage.setItem('voxintel-theme', theme);
}

(function initTheme() {
  const saved = localStorage.getItem('voxintel-theme');
  const preferred = saved || (window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark');
  applyTheme(preferred);
})();

els.themeToggle.addEventListener('click', () => {
  const current = document.documentElement.getAttribute('data-theme');
  applyTheme(current === 'dark' ? 'light' : 'dark');
});

// --- Errors ---

function showError(msg) {
  els.errorBanner.textContent = msg;
  els.errorBanner.classList.remove('hidden');
  setTimeout(() => els.errorBanner.classList.add('hidden'), 6000);
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// --- Microphone / audio input device list ---
// Populates the dropdown with every audio input the OS sees — laptop mic,
// USB audio interface, a line-in cable from a mixer, etc. Browsers hide
// device *names* until mic permission has been granted once, so we quickly
// request+release access first just to unlock the labels.

async function populateMicList() {
  try {
    const tempStream = await navigator.mediaDevices.getUserMedia({ audio: true });
    tempStream.getTracks().forEach((t) => t.stop());

    const devices = await navigator.mediaDevices.enumerateDevices();
    const mics = devices.filter((d) => d.kind === 'audioinput');

    els.micSelect.innerHTML = '';
    mics.forEach((d, i) => {
      const opt = document.createElement('option');
      opt.value = d.deviceId;
      opt.textContent = d.label || `Microphone ${i + 1}`;
      els.micSelect.appendChild(opt);
    });
  } catch (err) {
    console.error('Could not list audio devices', err);
  }
}

navigator.mediaDevices.addEventListener('devicechange', populateMicList);
populateMicList();

// --- Recording ---

async function startRecording() {
  let stream;
  const constraints = {
    audio: els.micSelect.value ? { deviceId: { exact: els.micSelect.value } } : true,
  };
  try {
    stream = await navigator.mediaDevices.getUserMedia(constraints);
  } catch (err) {
    showError('Microphone access denied or unavailable: ' + err.message);
    return;
  }

  recordedChunks = [];
  mediaRecorder = new MediaRecorder(stream, { mimeType: 'audio/webm' });

  mediaRecorder.ondataavailable = (e) => {
    if (e.data.size > 0) recordedChunks.push(e.data);
  };

  mediaRecorder.onstop = async () => {
    stream.getTracks().forEach((t) => t.stop());
    const blob = new Blob(recordedChunks, { type: 'audio/webm' });
    await createSessionAndUpload(blob);
  };

  mediaRecorder.start();
  els.startBtn.disabled = true;
  els.stopBtn.disabled = false;
  els.recDot.classList.remove('hidden');
  els.recStatus.textContent = 'Recording';
}

function stopRecording() {
  if (mediaRecorder && mediaRecorder.state !== 'inactive') {
    mediaRecorder.stop();
  }
  els.startBtn.disabled = false;
  els.stopBtn.disabled = true;
  els.recDot.classList.add('hidden');
  els.recStatus.textContent = 'Processing';
}

// --- Backend calls ---

async function createSessionAndUpload(blob) {
  try {
    const createResp = await fetch('/api/sessions', { method: 'POST' });
    if (!createResp.ok) throw new Error('Failed to create session');
    const session = await createResp.json();
    currentSessionId = session.id;

    const uploadResp = await fetch(`/api/sessions/${session.id}/audio`, {
      method: 'POST',
      headers: { 'Content-Type': 'audio/webm' },
      body: blob,
    });
    if (!uploadResp.ok) {
      const body = await uploadResp.json().catch(() => ({}));
      throw new Error(body.error || 'Upload failed');
    }

    els.recStatus.textContent = 'Idle';
    resetSessionPanel();
    els.sessionPanel.classList.remove('hidden');
    pollSession(session.id);
    loadSessionList();
  } catch (err) {
    showError(err.message);
    els.recStatus.textContent = 'Idle';
  }
}

function pollSession(id) {
  if (pollHandle) clearInterval(pollHandle);
  const poll = async () => {
    try {
      const resp = await fetch(`/api/sessions/${id}`);
      if (!resp.ok) throw new Error('Failed to fetch session status');
      const session = await resp.json();
      renderSession(session);
      if (session.status === 'transcribed' || session.status === 'failed') {
        clearInterval(pollHandle);
        pollHandle = null;
      }
    } catch (err) {
      showError(err.message);
      clearInterval(pollHandle);
      pollHandle = null;
    }
  };
  poll();
  pollHandle = setInterval(poll, 3000);
}

function resetSessionPanel() {
  els.transcript.textContent = 'Transcribing…';
  els.summaryBox.classList.add('hidden');
  els.summaryText.innerHTML = '';
  els.summaryBtn.disabled = true;
  els.exportBtn.disabled = true;
  els.deleteBtn.disabled = true;
}

function renderSession(session) {
  currentSessionId = session.id;
  els.sessionStatus.textContent = session.status;
  els.sessionStatus.className = 'status status-' + session.status;
  els.deleteBtn.disabled = false;

  if (session.status === 'transcribed') {
    els.transcript.textContent = session.transcript;
    els.summaryBtn.disabled = false;
    els.exportBtn.disabled = false;
  } else if (session.status === 'failed') {
    els.transcript.textContent = '—';
    showError('Transcription failed: ' + (session.error || 'unknown error'));
  } else {
    els.transcript.textContent = 'Transcribing…';
  }

  if (session.analyses && session.analyses.length > 0) {
    const latest = session.analyses[session.analyses.length - 1];
    renderTakeaways(latest.answer);
    els.summaryBox.classList.remove('hidden');
  }
}

// Renders the model's JSON takeaways response as a numbered card board.
// Falls back to plain text if the response isn't valid JSON for some
// reason, so a parsing hiccup never just shows a blank box.
function renderTakeaways(rawAnswer) {
  els.summaryText.innerHTML = '';
  let parsed;
  try {
    parsed = JSON.parse(rawAnswer);
  } catch {
    els.summaryText.textContent = rawAnswer;
    return;
  }

  const board = document.createElement('div');
  board.className = 'takeaway-board';
  (parsed.takeaways || []).forEach((t, i) => {
    const card = document.createElement('div');
    card.className = 'takeaway-card';
    card.innerHTML = `
      <span class="takeaway-num">${i + 1}</span>
      <div>
        <p class="takeaway-statement">${escapeHtml(t.statement)}</p>
        ${t.explanation ? `<p class="takeaway-explanation">${escapeHtml(t.explanation)}</p>` : ''}
      </div>`;
    board.appendChild(card);
  });
  els.summaryText.appendChild(board);
}

async function generateSummary() {
  if (!currentSessionId) return;
  els.summaryBtn.disabled = true;
  els.summaryBtn.textContent = 'Generating…';

  try {
    const resp = await fetch(`/api/sessions/${currentSessionId}/analyze`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt: FIXED_PROMPT }),
    });
    const body = await resp.json();
    if (!resp.ok) throw new Error(body.error || 'Summary generation failed');

    renderTakeaways(body.answer);
    els.summaryBox.classList.remove('hidden');
  } catch (err) {
    showError(err.message);
  } finally {
    els.summaryBtn.disabled = false;
    els.summaryBtn.textContent = 'Generate summary';
  }
}

function exportSession() {
  if (!currentSessionId) return;
  window.location.href = `/api/sessions/${currentSessionId}/export`;
}

async function deleteSession(id) {
  if (!confirm('Delete this recording and its transcript? This cannot be undone.')) return;
  try {
    const resp = await fetch(`/api/sessions/${id}`, { method: 'DELETE' });
    if (!resp.ok) throw new Error('Failed to delete session');
    if (currentSessionId === id) {
      currentSessionId = null;
      els.sessionPanel.classList.add('hidden');
    }
    loadSessionList();
  } catch (err) {
    showError(err.message);
  }
}

async function loadSessionList() {
  try {
    const resp = await fetch('/api/sessions');
    if (!resp.ok) return;
    const sessions = await resp.json();
    sessions.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));

    els.historyEmpty.classList.toggle('hidden', sessions.length > 0);
    els.sessionList.innerHTML = '';

    sessions.forEach((s) => {
      const li = document.createElement('li');
      const date = new Date(s.created_at).toLocaleString();

      const main = document.createElement('div');
      main.className = 'session-main';
      main.innerHTML = `<span class="session-date">${date}</span><span class="status status-${s.status}">${s.status}</span>`;
      main.addEventListener('click', () => {
        els.sessionPanel.classList.remove('hidden');
        resetSessionPanel();
        renderSession(s);
        if (s.status !== 'transcribed' && s.status !== 'failed') {
          pollSession(s.id);
        }
      });

      const del = document.createElement('button');
      del.className = 'session-delete';
      del.textContent = 'Delete';
      del.addEventListener('click', (e) => {
        e.stopPropagation();
        deleteSession(s.id);
      });

      li.appendChild(main);
      li.appendChild(del);
      els.sessionList.appendChild(li);
    });
  } catch (err) {
    console.error('Failed to load session list', err);
  }
}

// --- Wire up events ---

els.startBtn.addEventListener('click', startRecording);
els.stopBtn.addEventListener('click', stopRecording);
els.summaryBtn.addEventListener('click', generateSummary);
els.exportBtn.addEventListener('click', exportSession);
els.deleteBtn.addEventListener('click', () => currentSessionId && deleteSession(currentSessionId));

loadSessionList();
