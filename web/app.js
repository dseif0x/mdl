"use strict";

const els = {
  provider: document.getElementById("provider"),
  query: document.getElementById("query"),
  searchForm: document.getElementById("search-form"),
  urlProvider: document.getElementById("url-provider"),
  urlInput: document.getElementById("url-input"),
  urlForm: document.getElementById("url-form"),
  status: document.getElementById("status"),
  results: document.getElementById("results"),
  jobsPanel: document.getElementById("jobs-panel"),
  jobs: document.getElementById("jobs"),
};

// setStatus shows a transient message; kind controls the colour.
function setStatus(message, kind = "info") {
  els.status.textContent = message || "";
  els.status.dataset.kind = kind;
}

async function api(path, options) {
  const res = await fetch(path, options);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `request failed (${res.status})`);
  }
  return data;
}

function formatDuration(seconds) {
  if (!seconds) return "";
  const m = Math.floor(seconds / 60);
  const s = String(seconds % 60).padStart(2, "0");
  return `${m}:${s}`;
}

async function loadProviders() {
  const { providers } = await api("/api/providers");
  const searchable = providers.filter((p) => p.capabilities.search);
  for (const p of searchable) {
    els.provider.add(new Option(p.display_name, p.name));
  }
  for (const p of providers) {
    els.urlProvider.add(new Option(p.display_name, p.name));
  }
  if (searchable.length === 0) {
    setStatus("No searchable providers configured.", "error");
  }
}

function renderResults(tracks) {
  els.results.innerHTML = "";
  if (!tracks || tracks.length === 0) {
    setStatus("No results.", "info");
    return;
  }
  for (const track of tracks) {
    els.results.appendChild(renderTrack(track));
  }
}

function renderTrack(track) {
  const li = document.createElement("li");
  li.className = "track";

  if (track.artwork_url) {
    const img = document.createElement("img");
    img.className = "art";
    img.src = track.artwork_url;
    img.alt = "";
    img.loading = "lazy";
    li.appendChild(img);
  }

  const meta = document.createElement("div");
  meta.className = "meta";
  const title = document.createElement("span");
  title.className = "title";
  title.textContent = track.title || "(untitled)";
  const sub = document.createElement("span");
  sub.className = "sub";
  sub.textContent = [track.artist, track.album].filter(Boolean).join(" — ");
  meta.append(title, sub);
  li.appendChild(meta);

  const dur = document.createElement("span");
  dur.className = "dur";
  dur.textContent = formatDuration(track.duration);
  li.appendChild(dur);

  const btn = document.createElement("button");
  btn.className = "dl";
  btn.textContent = "Download";
  btn.addEventListener("click", () => downloadTrack(track, btn));
  li.appendChild(btn);

  return li;
}

async function enqueueDownload(body) {
  const job = await api("/api/download", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  startJobPolling();
  return job;
}

async function downloadTrack(track, btn) {
  btn.disabled = true;
  btn.textContent = "Queued…";
  try {
    await enqueueDownload({ provider: track.provider, track });
    setStatus(`Queued "${track.title}".`, "success");
    btn.textContent = "Queued ✓";
    setTimeout(() => {
      btn.disabled = false;
      btn.textContent = "Download";
    }, 1500);
  } catch (err) {
    setStatus(err.message, "error");
    btn.textContent = "Retry";
    btn.disabled = false;
  }
}

// --- downloads panel -------------------------------------------------------

const JOB_LABELS = {
  queued: "Queued",
  running: "Downloading…",
  completed: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
};

let jobsTimer = null;

function startJobPolling() {
  refreshJobs();
  if (jobsTimer === null) {
    jobsTimer = setInterval(refreshJobs, 1500);
  }
}

async function refreshJobs() {
  let jobs;
  try {
    ({ jobs } = await api("/api/jobs"));
  } catch {
    return; // transient; try again on the next tick
  }
  renderJobs(jobs);
  const active = jobs.some((j) => j.status === "queued" || j.status === "running");
  if (!active && jobsTimer !== null) {
    clearInterval(jobsTimer);
    jobsTimer = null;
  }
}

function renderJobs(jobs) {
  els.jobsPanel.hidden = jobs.length === 0;
  els.jobs.innerHTML = "";
  for (const job of jobs) {
    els.jobs.appendChild(renderJob(job));
  }
}

function renderJob(job) {
  const li = document.createElement("li");
  li.className = "job";
  li.dataset.status = job.status;

  const meta = document.createElement("div");
  meta.className = "meta";
  const title = document.createElement("span");
  title.className = "title";
  title.textContent = job.track.title || job.track.url;
  const sub = document.createElement("span");
  sub.className = "sub";
  sub.textContent = `${job.provider} · ${JOB_LABELS[job.status] || job.status}`;
  if (job.status === "failed" && job.error) sub.textContent += ` — ${job.error}`;
  meta.append(title, sub);
  li.appendChild(meta);

  if (job.status === "queued" || job.status === "running") {
    const cancel = document.createElement("button");
    cancel.className = "cancel";
    cancel.textContent = "Cancel";
    cancel.addEventListener("click", async () => {
      cancel.disabled = true;
      try {
        await api(`/api/jobs/${job.id}/cancel`, { method: "POST" });
        refreshJobs();
      } catch (err) {
        setStatus(err.message, "error");
        cancel.disabled = false;
      }
    });
    li.appendChild(cancel);
  }
  return li;
}

els.searchForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const q = els.query.value.trim();
  if (!q) return;
  setStatus("Searching…", "info");
  els.results.innerHTML = "";
  try {
    const params = new URLSearchParams({ provider: els.provider.value, q });
    const { tracks } = await api(`/api/search?${params}`);
    setStatus("");
    renderResults(tracks);
  } catch (err) {
    setStatus(err.message, "error");
  }
});

els.urlForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const url = els.urlInput.value.trim();
  if (!url) return;
  try {
    await enqueueDownload({ provider: els.urlProvider.value, url });
    setStatus("Download queued.", "success");
    els.urlInput.value = "";
  } catch (err) {
    setStatus(err.message, "error");
  }
});

loadProviders().catch((err) => setStatus(`Failed to load providers: ${err.message}`, "error"));
// Surface any jobs already in progress (e.g. after a page reload).
refreshJobs();
