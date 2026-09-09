const els = {
  startBtn: document.getElementById('startBtn'),
  stopBtn: document.getElementById('stopBtn'),
  recStatus: document.getElementById('recStatus'),
  sessionPanel: document.getElementById('sessionPanel'),
  sessionStatus: document.getElementById('sessionStatus'),
  transcript: document.getElementById('transcript'),
  promptInput: document.getElementById('promptInput'),
  askBtn: document.getElementById('askBtn'),
  analyses: document.getElementById('analyses'),
  exportBtn: document.getElementById('exportBtn'),
  sessionList: document.getElementById('sessionList'),
  errorBanner: document.getElementById('errorBanner'),
};

let mediaRecorder = null;
let recordedChunks = [];
let currentSessionId = null;
let pollHandle = null;

function showError(msg) {
  els.errorBanner.textContent = msg;
  els.errorBanner.classList.remove('hidden');
  setTimeout(() => els.errorBanner.classList.add('hidden'), 6000);
}

// --- Recording ---

async function startRecording() {
  let stream;
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true });
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
    stream.getTracks().forEach((t) => t.stop()); // release the mic
    const blob = new Blob(recordedChunks, { type: 'audio/webm' });
    await createSessionAndUpload(blob);
  };

  mediaRecorder.start();
  els.startBtn.disabled = true;
  els.stopBtn.disabled = false;
  els.recStatus.textContent = 'Recording...';
}

function stopRecording() {
  if (mediaRecorder && mediaRecorder.state !== 'inactive') {
    mediaRecorder.stop();
  }
  els.startBtn.disabled = false;
  els.stopBtn.disabled = true;
  els.recStatus.textContent = 'Processing...';
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

    els.recStatus.textContent = 'Uploaded. Transcribing...';
    showSessionPanel();
    pollSession(session.id);
    loadSessionList();
  } catch (err) {
    showError(err.message);
    els.recStatus.textContent = 'Idle';
  }
}

function pollSession(id) {
  if (pollHandle) clearInterval(pollHandle);
  pollHandle = setInterval(async () => {
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
  }, 3000);
}

function renderSession(session) {
  currentSessionId = session.id;
  els.sessionStatus.textContent = session.status;
  els.sessionStatus.className = 'status status-' + session.status;

  if (session.status === 'transcribed') {
    els.transcript.textContent = session.transcript;
    els.askBtn.disabled = false;
    els.exportBtn.disabled = false;
    els.recStatus.textContent = 'Idle';
  } else if (session.status === 'failed') {
    showError('Transcription failed: ' + (session.error || 'unknown error'));
    els.recStatus.textContent = 'Idle';
  } else {
    els.transcript.textContent = '(transcribing...)';
  }

  els.analyses.innerHTML = '';
  (session.analyses || []).forEach((a) => {
    const item = document.createElement('div');
    item.className = 'analysis-item';
    item.innerHTML = `<div class="analysis-q">Q: ${escapeHtml(a.prompt)}</div><div class="analysis-a">${escapeHtml(a.answer)}</div>`;
    els.analyses.prepend(item);
  });
}

async function askPrompt() {
  const prompt = els.promptInput.value.trim();
  if (!prompt || !currentSessionId) return;

  els.askBtn.disabled = true;
  try {
    const resp = await fetch(`/api/sessions/${currentSessionId}/analyze`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt }),
    });
    const body = await resp.json();
    if (!resp.ok) throw new Error(body.error || 'Analysis failed');

    const item = document.createElement('div');
    item.className = 'analysis-item';
    item.innerHTML = `<div class="analysis-q">Q: ${escapeHtml(body.prompt)}</div><div class="analysis-a">${escapeHtml(body.answer)}</div>`;
    els.analyses.prepend(item);
    els.promptInput.value = '';
  } catch (err) {
    showError(err.message);
  } finally {
    els.askBtn.disabled = false;
  }
}

function exportSession() {
  if (!currentSessionId) return;
  window.location.href = `/api/sessions/${currentSessionId}/export`;
}

async function loadSessionList() {
  try {
    const resp = await fetch('/api/sessions');
    if (!resp.ok) return;
    const sessions = await resp.json();
    sessions.sort((a, b) => new Date(b.created_at) - new Date(a.created_at));

    els.sessionList.innerHTML = '';
    sessions.forEach((s) => {
      const li = document.createElement('li');
      const date = new Date(s.created_at).toLocaleString();
      li.innerHTML = `<span>${date}</span><span class="status status-${s.status}">${s.status}</span>`;
      li.addEventListener('click', () => {
        showSessionPanel();
        renderSession(s);
        if (s.status !== 'transcribed' && s.status !== 'failed') {
          pollSession(s.id);
        }
      });
      els.sessionList.appendChild(li);
    });
  } catch (err) {
    // Non-critical: session list is a convenience, don't block the UI on it
    console.error('Failed to load session list', err);
  }
}

function showSessionPanel() {
  els.sessionPanel.classList.remove('hidden');
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// --- Wire up events ---

els.startBtn.addEventListener('click', startRecording);
els.stopBtn.addEventListener('click', stopRecording);
els.askBtn.addEventListener('click', askPrompt);
els.exportBtn.addEventListener('click', exportSession);
els.promptInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') askPrompt();
});

loadSessionList();
