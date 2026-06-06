"use strict";

const els = {
  provider: document.getElementById("provider"),
  type: document.getElementById("type"),
  query: document.getElementById("query"),
  searchForm: document.getElementById("search-form"),
  urlProvider: document.getElementById("url-provider"),
  urlInput: document.getElementById("url-input"),
  urlForm: document.getElementById("url-form"),
  breadcrumb: document.getElementById("breadcrumb"),
  status: document.getElementById("status"),
  results: document.getElementById("results"),
  jobsPanel: document.getElementById("jobs-panel"),
  jobs: document.getElementById("jobs"),
};

// providers maps name -> provider info (incl. capabilities).
const providers = new Map();
// nav is a stack of { label, items } levels: [search, album, ...].
let nav = [];

function setStatus(message, kind = "info") {
  els.status.textContent = message || "";
  els.status.dataset.kind = kind;
}

async function api(path, options) {
  const res = await fetch(path, options);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `request failed (${res.status})`);
  return data;
}

function formatDuration(seconds) {
  if (!seconds) return "";
  const m = Math.floor(seconds / 60);
  return `${m}:${String(seconds % 60).padStart(2, "0")}`;
}

// --- providers & search types ---------------------------------------------

async function loadProviders() {
  const data = await api("/api/providers");
  for (const p of data.providers) {
    providers.set(p.name, p);
    if (p.capabilities.search) els.provider.add(new Option(p.display_name, p.name));
    if (p.capabilities.download) els.urlProvider.add(new Option(p.display_name, p.name));
  }
  if (els.provider.options.length === 0) {
    setStatus("No searchable providers configured.", "error");
    return;
  }
  updateTypeOptions();
  els.provider.addEventListener("change", updateTypeOptions);
}

// updateTypeOptions rebuilds the Songs/Albums/Artists choices for the selected
// provider based on its advertised capabilities.
function updateTypeOptions() {
  const caps = providers.get(els.provider.value)?.capabilities ?? {};
  const opts = [];
  if (caps.search) opts.push(["song", "Songs"]);
  if (caps.search_albums) opts.push(["album", "Albums"]);
  if (caps.search_artists) opts.push(["artist", "Artists"]);
  els.type.innerHTML = "";
  for (const [value, label] of opts) els.type.add(new Option(label, value));
  els.type.disabled = opts.length <= 1;
}

// --- search & browse ------------------------------------------------------

els.searchForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const q = els.query.value.trim();
  if (!q) return;
  const type = els.type.value || "song";
  setStatus("Searching…");
  els.results.innerHTML = "";
  try {
    const params = new URLSearchParams({ provider: els.provider.value, q, type });
    const { items } = await api(`/api/search?${params}`);
    const label = `${els.type.selectedOptions[0]?.text || "Results"}: ${q}`;
    nav = [{ label, items }];
    render();
  } catch (err) {
    setStatus(err.message, "error");
  }
});

// open browses into an album (its tracks) or artist (its albums).
async function open(item) {
  setStatus("Loading…");
  try {
    const params = new URLSearchParams({
      provider: item.provider,
      kind: item.kind,
      id: item.id || "",
      url: item.url || "",
      title: item.title || "",
    });
    const { items } = await api(`/api/browse?${params}`);
    nav.push({ label: item.title, items });
    render();
  } catch (err) {
    setStatus(err.message, "error");
  }
}

// --- rendering ------------------------------------------------------------

function render() {
  setStatus("");
  renderBreadcrumb();
  const level = nav[nav.length - 1];
  els.results.innerHTML = "";
  if (!level || level.items.length === 0) {
    setStatus("No results.", "info");
    return;
  }
  for (const item of level.items) els.results.appendChild(renderItem(item));
}

function renderBreadcrumb() {
  els.breadcrumb.hidden = nav.length <= 1;
  els.breadcrumb.innerHTML = "";
  nav.forEach((level, i) => {
    if (i > 0) els.breadcrumb.append(Object.assign(document.createElement("span"), { className: "sep", textContent: "›" }));
    const crumb = document.createElement("button");
    crumb.className = "crumb";
    crumb.textContent = level.label;
    crumb.disabled = i === nav.length - 1;
    crumb.addEventListener("click", () => {
      nav = nav.slice(0, i + 1);
      render();
    });
    els.breadcrumb.appendChild(crumb);
  });
}

function renderItem(item) {
  const li = document.createElement("li");
  li.className = "item";
  li.dataset.kind = item.kind || "track";

  if (item.artwork_url) {
    const img = document.createElement("img");
    img.className = "art";
    img.src = item.artwork_url;
    img.alt = "";
    img.loading = "lazy";
    li.appendChild(img);
  }

  const meta = document.createElement("div");
  meta.className = "meta";
  const title = document.createElement("span");
  title.className = "title";
  title.textContent = item.title || "(untitled)";
  const sub = document.createElement("span");
  sub.className = "sub";
  sub.textContent = subtitle(item);
  meta.append(title, sub);
  li.appendChild(meta);

  if (item.kind === "track" && item.duration) {
    li.appendChild(Object.assign(document.createElement("span"), {
      className: "dur",
      textContent: formatDuration(item.duration),
    }));
  }

  const actions = document.createElement("div");
  actions.className = "actions";
  const browseable = item.kind === "album" || item.kind === "artist";
  if (browseable && providers.get(item.provider)?.capabilities.browse) {
    const openBtn = document.createElement("button");
    openBtn.className = "open";
    openBtn.textContent = "Open";
    openBtn.addEventListener("click", () => open(item));
    actions.appendChild(openBtn);
  }
  const dlBtn = document.createElement("button");
  dlBtn.className = "dl";
  dlBtn.textContent = downloadLabel(item.kind);
  dlBtn.addEventListener("click", () => download(item, dlBtn));
  actions.appendChild(dlBtn);
  li.appendChild(actions);

  return li;
}

function subtitle(item) {
  if (item.kind === "artist") return "Artist";
  if (item.kind === "album") {
    const parts = [item.artist, item.year, item.track_count ? `${item.track_count} tracks` : ""];
    return parts.filter(Boolean).join(" · ");
  }
  return [item.artist, item.album].filter(Boolean).join(" — ");
}

function downloadLabel(kind) {
  if (kind === "album") return "Download album";
  if (kind === "artist") return "Download all";
  return "Download";
}

// --- downloads ------------------------------------------------------------

async function download(item, btn) {
  btn.disabled = true;
  btn.textContent = "Queued…";
  try {
    await api("/api/download", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ provider: item.provider, track: item }),
    });
    setStatus(`Queued "${item.title}".`, "success");
    startJobPolling();
    btn.textContent = "Queued ✓";
    setTimeout(() => {
      btn.disabled = false;
      btn.textContent = downloadLabel(item.kind);
    }, 1500);
  } catch (err) {
    setStatus(err.message, "error");
    btn.disabled = false;
    btn.textContent = "Retry";
  }
}

els.urlForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const url = els.urlInput.value.trim();
  if (!url) return;
  try {
    await api("/api/download", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ provider: els.urlProvider.value, url }),
    });
    setStatus("Download queued.", "success");
    startJobPolling();
    els.urlInput.value = "";
  } catch (err) {
    setStatus(err.message, "error");
  }
});

// --- downloads panel ------------------------------------------------------

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
  if (jobsTimer === null) jobsTimer = setInterval(refreshJobs, 1500);
}

async function refreshJobs() {
  let jobs;
  try {
    ({ jobs } = await api("/api/jobs"));
  } catch {
    return;
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
  for (const job of jobs) els.jobs.appendChild(renderJob(job));
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

loadProviders().catch((err) => setStatus(`Failed to load providers: ${err.message}`, "error"));
refreshJobs();
