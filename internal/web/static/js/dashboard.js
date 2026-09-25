// BeastDB Studio Dashboard Controller

function showToast(msg, type = "success") {
  const container = document.getElementById("toastContainer");
  const toast = document.createElement("div");
  toast.className = `toast toast-${type}`;
  toast.textContent = msg;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3500);
}

function updateInspector(data, isError = false) {
  const el = document.getElementById("recordInspector");
  if (typeof data === "object") {
    el.textContent = JSON.stringify(data, null, 2);
  } else {
    try {
      const parsed = JSON.parse(data);
      el.textContent = JSON.stringify(parsed, null, 2);
    } catch {
      el.textContent = data;
    }
  }
  el.style.color = isError ? "#f87171" : "#38bdf8";
}

async function fetchTelemetry() {
  try {
    const res = await fetch("/api/stats");
    if (!res.ok) return;
    const data = await res.json();
    document.getElementById("statLsn").textContent = `#${data.lsn ?? 0}`;
    document.getElementById("statRole").textContent = data.role ?? "Leader";
    document.getElementById("statMode").textContent = data.mode ?? "B+ Tree / WAL";
  } catch (err) {
    console.debug("Telemetry fetch paused:", err);
  }
}

async function executeGet() {
  const keyInput = document.getElementById("searchKey").value.trim();
  if (!keyInput) {
    showToast("Please enter a numeric key", "error");
    return;
  }
  try {
    const res = await fetch(`/api/key?k=${encodeURIComponent(keyInput)}`);
    const body = await res.text();
    if (res.ok) {
      updateInspector(body);
      showToast(`Key ${keyInput} loaded successfully`, "success");
    } else {
      updateInspector(`HTTP ${res.status}: ${body}`, true);
      showToast(`Key ${keyInput} not found (${res.status})`, "error");
    }
  } catch {
    showToast("Network error executing GET", "error");
  }
}

async function executePut() {
  const key = document.getElementById("mutationKey").value.trim();
  const rawVal = document.getElementById("mutationVal").value.trim();
  if (!key) {
    showToast("Key is required for insert/update", "error");
    return;
  }
  try {
    const res = await fetch("/api/key", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: Number(key), value: rawVal }),
    });
    if (res.ok) {
      showToast(`Key ${key} committed to WAL & B+ Tree`, "success");
      updateInspector({ status: "COMMITTED", key: Number(key), payload: rawVal });
      fetchTelemetry();
    } else {
      const err = await res.text();
      showToast(`Write error: ${err}`, "error");
    }
  } catch {
    showToast("Network error executing write", "error");
  }
}

async function executeDelete() {
  const key = document.getElementById("searchKey").value.trim();
  if (!key) {
    showToast("Enter a key to delete in the search field", "error");
    return;
  }
  if (!confirm(`Are you sure you want to delete Key ${key}?`)) return;
  try {
    const res = await fetch(`/api/key?k=${encodeURIComponent(key)}`, { method: "DELETE" });
    if (res.ok) {
      showToast(`Key ${key} deleted with WAL tombstone`, "success");
      updateInspector({ status: "DELETED", key: Number(key) });
      fetchTelemetry();
    } else {
      showToast(`Delete failed (${res.status})`, "error");
    }
  } catch {
    showToast("Network error executing delete", "error");
  }
}

function formatJsonInput() {
  const textarea = document.getElementById("mutationVal");
  try {
    const parsed = JSON.parse(textarea.value);
    textarea.value = JSON.stringify(parsed, null, 2);
  } catch {
    showToast("Value is not valid JSON", "error");
  }
}

function copyInspector() {
  const text = document.getElementById("recordInspector").textContent;
  navigator.clipboard.writeText(text);
  showToast("Inspector content copied to clipboard!", "success");
}

document.addEventListener("DOMContentLoaded", () => {
  fetchTelemetry();
  setInterval(fetchTelemetry, 3000);
});
