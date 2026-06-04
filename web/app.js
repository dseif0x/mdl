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

async function downloadTrack(track, btn) {
  btn.disabled = true;
  btn.textContent = "Downloading…";
  setStatus(`Downloading "${track.title}"…`, "info");
  try {
    const result = await api("/api/download", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ provider: track.provider, track }),
    });
    const count = (result.files || []).length;
    setStatus(count ? `Saved ${count} file(s).` : "Download complete.", "success");
    btn.textContent = "Done ✓";
  } catch (err) {
    setStatus(err.message, "error");
    btn.textContent = "Retry";
    btn.disabled = false;
  }
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
  setStatus("Downloading…", "info");
  try {
    const result = await api("/api/download", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ provider: els.urlProvider.value, url }),
    });
    const count = (result.files || []).length;
    setStatus(count ? `Saved ${count} file(s).` : "Download complete.", "success");
    els.urlInput.value = "";
  } catch (err) {
    setStatus(err.message, "error");
  }
});

loadProviders().catch((err) => setStatus(`Failed to load providers: ${err.message}`, "error"));
