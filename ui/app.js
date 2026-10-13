const state = {
  catalog: null,
  environments: [],
  envId: "",
  endpoint: null,
  bodyMode: "form",
  lastResponse: null,
  lastRaw: "",
  responseTab: "body",
  side: "apis",
  saved: [],
  sequences: [],
  sequence: { name: "", steps: [] },
  selectedStep: -1,
  stepSeq: 0,
  variables: {},
  fileBody: "",
  fileName: "",
  fileType: "",
};

const $ = (id) => document.getElementById(id);

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { raw: text }; }
  if (!res.ok) {
    const msg = (data && (data.error || data.message)) || res.statusText;
    throw new Error(msg);
  }
  return data;
}

function methodClass(m) { return `method ${(m || "").toUpperCase()}`; }

function currentEnv() {
  return state.environments.find((e) => e.id === state.envId) || state.environments[0];
}

function selectedAuthMode() {
  const el = document.querySelector("input[name=auth-mode]:checked");
  return el ? el.value : "jwt";
}

function setAuthMode(mode) {
  const el = document.querySelector(`input[name=auth-mode][value="${mode || "jwt"}"]`);
  if (el) el.checked = true;
  const current = selectedAuthMode();
  $("jwt-fields").classList.toggle("hidden", current === "custom");
  $("custom-auth-wrap").classList.toggle("hidden", current !== "custom");
}

function note(msg) { $("action-note").textContent = msg || ""; }
function seqNote(msg) { $("seq-note").textContent = msg || ""; }

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

async function copy(text) {
  try {
    await navigator.clipboard.writeText(text || "");
  } catch {
    note("Copy failed");
  }
}

function renderEnvironments(payload) {
  state.environments = payload.environments || [];
  state.envId = payload.default || (state.environments[0] && state.environments[0].id) || "";
  const sel = $("environment");
  sel.innerHTML = state.environments.map((e) =>
    `<option value="${escapeHtml(e.id)}" ${e.id === state.envId ? "selected" : ""}>${escapeHtml(e.name)}</option>`
  ).join("");
  applyEnv();
}

function applyEnv() {
  const env = currentEnv();
  if (!env) return;
  $("prod-badge").classList.toggle("hidden", !env.production);
  $("base-url").value = env.base_url || "";
  if (env.client_id && !$("client-id").value) $("client-id").value = env.client_id;
  $("username").placeholder = env.has_stored_credentials ? "using server credentials" : "Cognito username / email";
  $("password").placeholder = env.has_stored_credentials ? "using server credentials" : "Cognito password";
}

function setSide(side) {
  state.side = side;
  document.querySelectorAll("[data-side]").forEach((b) => b.classList.toggle("active", b.dataset.side === side));
  $("api-search").classList.toggle("hidden", side !== "apis");
  $("endpoint-list").classList.toggle("hidden", side !== "apis");
  $("saved-list").classList.toggle("hidden", side !== "saved");
  $("sequence-list").classList.toggle("hidden", side !== "sequences");
}

function renderCatalog(catalog) {
  state.catalog = catalog;
  $("spec-title").textContent = `${catalog.title || "OpenAPI"} · ${catalog.endpoints.length} endpoints`;
  renderEndpointList();
}

function renderEndpointList() {
  const q = ($("api-search").value || "").toLowerCase().trim();
  const groups = new Map();
  for (const ep of state.catalog.endpoints) {
    const hay = `${ep.method} ${ep.path} ${ep.summary || ""} ${ep.operation_id || ""} ${(ep.tags || []).join(" ")}`.toLowerCase();
    if (q && !hay.includes(q)) continue;
    const tag = (ep.tags && ep.tags[0]) || "untagged";
    if (!groups.has(tag)) groups.set(tag, []);
    groups.get(tag).push(ep);
  }
  const nav = $("endpoint-list");
  if (!groups.size) {
    nav.innerHTML = `<p class="muted">No matching APIs.</p>`;
    return;
  }
  nav.innerHTML = [...groups.entries()].map(([tag, eps]) => `
    <div class="tag-group">
      <h4>${escapeHtml(tag)}</h4>
      ${eps.map((ep) => `
        <button type="button" class="ep ${state.endpoint && state.endpoint.id === ep.id ? "active" : ""}" data-id="${encodeURIComponent(ep.id)}">
          <span class="${methodClass(ep.method)}">${escapeHtml(ep.method)}</span>
          <span class="path">${escapeHtml(ep.path)}</span>
        </button>
      `).join("")}
    </div>
  `).join("");
  nav.querySelectorAll(".ep").forEach((btn) => {
    btn.addEventListener("click", () => {
      const id = decodeURIComponent(btn.dataset.id);
      selectEndpoint(state.catalog.endpoints.find((e) => e.id === id));
    });
  });
}

function setRequestButtons(on) {
  ["btn-send", "btn-save-request", "btn-fill-example", "btn-copy-curl", "btn-copy-call"].forEach((id) => {
    $(id).disabled = !on;
  });
}

function selectEndpoint(ep) {
  state.endpoint = ep;
  state.fileBody = "";
  state.fileName = "";
  state.fileType = "";
  $("file-name").textContent = "No file chosen. The file is sent as text.";
  clearExtraHeaders();
  setRequestButtons(!!ep);
  renderEndpointList();
  if (!ep) return;

  $("op-title").textContent = ep.summary || ep.operation_id || ep.id;
  $("op-desc").textContent = ep.description || "";
  $("op-method").textContent = ep.method;
  $("op-method").className = methodClass(ep.method);
  $("op-path").textContent = ep.path;
  $("op-auth").classList.remove("hidden");
  $("op-auth").textContent = ep.auth_required ? "Auth required" : "Public";
  $("op-auth").className = "chip " + (ep.auth_required ? "warn" : "ok");

  renderParams(ep);
  renderBody(ep);
}

function renderParams(ep) {
  const wrap = $("params-wrap");
  const box = $("params");
  const params = ep.parameters || [];
  if (!params.length) {
    wrap.classList.add("hidden");
    box.innerHTML = "";
    return;
  }
  wrap.classList.remove("hidden");
  box.innerHTML = params.map((p) => {
    const ex = p.example != null ? String(p.example) : "";
    return `<label class="param">
      <span class="name">${escapeHtml(p.name)}${p.required ? ' <span class="req">*</span>' : ""} <small class="muted">${escapeHtml(p.in)}</small></span>
      <input data-param-in="${escapeHtml(p.in)}" data-param-name="${escapeHtml(p.name)}" placeholder="${escapeHtml(ex || p.description || p.name)}" spellcheck="false" />
    </label>`;
  }).join("");
}

function schemaFields(schema) {
  if (!schema) return [];
  if (schema.type === "object" || schema.properties) {
    const req = new Set(schema.required || []);
    return Object.entries(schema.properties || {}).map(([name, prop]) => ({
      key: name,
      required: req.has(name),
      schema: prop,
    }));
  }
  return [];
}

function renderBody(ep) {
  const wrap = $("body-wrap");
  wrap.classList.remove("hidden");
  const fields = ep.request_body ? schemaFields(ep.request_body.schema) : [];
  const formBtn = document.querySelector('[data-body-mode="form"]');
  formBtn.classList.toggle("hidden", !fields.length);
  const example = ep.request_body
    ? (ep.request_body.example_json || (ep.request_body.example ? JSON.stringify(ep.request_body.example, null, 2) : "{\n  \n}"))
    : "";
  $("body-json").value = example;
  $("body-text").value = "";
  $("body-form").innerHTML = fields.map((f) => {
    const t = (f.schema && f.schema.type) || "string";
    const inputType = t === "integer" || t === "number" ? "number" : (f.schema && f.schema.format === "password" ? "password" : "text");
    const ex = f.schema && f.schema.example != null ? String(f.schema.example) : "";
    return `<label class="param">
      <span class="name">${escapeHtml(f.key)}${f.required ? ' <span class="req">*</span>' : ""}</span>
      <input data-form-key="${escapeHtml(f.key)}" type="${inputType}" placeholder="${escapeHtml(ex || t)}" spellcheck="false" />
    </label>`;
  }).join("");
  let mode = state.bodyMode;
  if (!fields.length && mode === "form") mode = "json";
  if (!ep.request_body && mode === "form") mode = "json";
  setBodyMode(mode);
}

function setBodyMode(mode) {
  state.bodyMode = mode;
  document.querySelectorAll("[data-body-mode]").forEach((b) => {
    b.classList.toggle("active", b.dataset.bodyMode === mode);
  });
  $("body-form").classList.toggle("hidden", mode !== "form");
  $("body-json-wrap").classList.toggle("hidden", mode !== "json");
  $("body-text-wrap").classList.toggle("hidden", mode !== "text");
  $("body-file-wrap").classList.toggle("hidden", mode !== "file");
}

function bodyFromForm() {
  const obj = {};
  $("body-form").querySelectorAll("[data-form-key]").forEach((el) => {
    if (el.value === "") return;
    obj[el.dataset.formKey] = el.type === "number" ? Number(el.value) : el.value;
  });
  return Object.keys(obj).length ? JSON.stringify(obj, null, 2) : ($("body-json").value || "");
}

function typeFromName(name) {
  const n = (name || "").toLowerCase();
  if (n.endsWith(".json")) return "application/json";
  if (n.endsWith(".txt")) return "text/plain; charset=utf-8";
  if (n.endsWith(".xml")) return "application/xml";
  if (n.endsWith(".yaml") || n.endsWith(".yml")) return "application/yaml";
  return "application/octet-stream";
}

function clearExtraHeaders() {
  $("extra-headers").innerHTML = "";
}

function addHeaderRow(name = "", value = "") {
  const row = document.createElement("div");
  row.className = "header-row";
  row.innerHTML = `
    <input data-h-name placeholder="Header" spellcheck="false" value="${escapeHtml(name)}" />
    <input data-h-value placeholder="Value" spellcheck="false" value="${escapeHtml(value)}" />
    <button type="button" class="ghost" data-h-del>Remove</button>`;
  row.querySelector("[data-h-del]").addEventListener("click", () => row.remove());
  $("extra-headers").appendChild(row);
}

function collectHeaders() {
  const headers = {};
  document.querySelectorAll("[data-param-in]").forEach((el) => {
    if (el.dataset.paramIn === "header" && el.value) headers[el.dataset.paramName] = el.value;
  });
  document.querySelectorAll(".header-row").forEach((row) => {
    const name = row.querySelector("[data-h-name]").value.trim();
    const value = row.querySelector("[data-h-value]").value;
    if (name) headers[name] = value;
  });
  return headers;
}

function collectRequest() {
  const ep = state.endpoint;
  const pathParams = {};
  const query = {};
  document.querySelectorAll("[data-param-in]").forEach((el) => {
    if (!el.value) return;
    if (el.dataset.paramIn === "path") pathParams[el.dataset.paramName] = el.value;
    else if (el.dataset.paramIn === "query") query[el.dataset.paramName] = el.value;
  });
  let body = "";
  let bodyType = "json";
  let contentType = "";
  if (state.bodyMode === "form") {
    body = bodyFromForm();
    bodyType = "json";
  } else if (state.bodyMode === "text") {
    body = $("body-text").value;
    bodyType = "text";
    contentType = "text/plain; charset=utf-8";
  } else if (state.bodyMode === "file") {
    body = state.fileBody || "";
    bodyType = "file";
    contentType = state.fileType || "application/octet-stream";
  } else {
    body = $("body-json").value;
    bodyType = "json";
  }
  if ((state.bodyMode === "form" || state.bodyMode === "json") && ep && ep.request_body && ep.request_body.content_type) {
    contentType = ep.request_body.content_type;
  }
  return {
    environment: state.envId,
    base_url: $("base-url").value.trim(),
    method: ep ? ep.method : "",
    path: ep ? ep.path : "",
    path_params: pathParams,
    query,
    headers: collectHeaders(),
    body,
    body_type: bodyType,
    content_type: contentType,
    auth_mode: selectedAuthMode(),
    jwt: $("jwt").value.trim(),
    custom_authorization: $("custom-auth").value.trim(),
  };
}

function fillParams(item) {
  document.querySelectorAll("[data-param-in]").forEach((el) => {
    const bag = el.dataset.paramIn === "path" ? item.path_params
      : el.dataset.paramIn === "query" ? item.query
      : item.headers;
    el.value = (bag && bag[el.dataset.paramName]) || "";
  });
}

function extraHeaderEntries(item, ep) {
  const spec = new Set((ep && ep.parameters || []).filter((p) => p.in === "header").map((p) => p.name.toLowerCase()));
  return Object.entries(item.headers || {}).filter(([k]) => !spec.has(k.toLowerCase()));
}

function applyCall(item) {
  const ep = state.catalog.endpoints.find((e) => e.method === item.method && e.path === item.path);
  if (ep) selectEndpoint(ep);
  if (item.environment) {
    state.envId = item.environment;
    $("environment").value = item.environment;
    applyEnv();
  }
  if (item.auth_mode) setAuthMode(item.auth_mode);
  if (ep) fillParams(item);
  clearExtraHeaders();
  extraHeaderEntries(item, ep).forEach(([name, value]) => addHeaderRow(name, value));
  const mode = item.body_type === "text" ? "text" : item.body_type === "file" ? "file" : (item.body ? "json" : state.bodyMode);
  if (item.body_type === "text" || (mode === "text" && item.body)) {
    $("body-text").value = item.body || "";
    setBodyMode("text");
  } else if (item.body_type === "file") {
    state.fileBody = item.body || "";
    state.fileType = item.content_type || typeFromName("file.txt");
    $("file-name").textContent = `Saved file body · ${state.fileBody.length} characters`;
    setBodyMode("file");
  } else if (item.body) {
    $("body-json").value = item.body;
    setBodyMode("json");
  }
}

async function confirmProduction(detail) {
  const env = currentEnv();
  if (!env || !env.production) return true;
  const dialog = $("prod-dialog");
  $("prod-dialog-detail").textContent = detail;
  dialog.showModal();
  return new Promise((resolve) => {
    dialog.addEventListener("close", function onClose() {
      dialog.removeEventListener("close", onClose);
      resolve(dialog.returnValue === "confirm");
    });
  });
}

async function sendRequest() {
  if (!state.endpoint) return;
  if (state.bodyMode === "json" && $("body-json").value.trim()) {
    try { JSON.parse($("body-json").value); $("json-error").textContent = ""; }
    catch (err) { $("json-error").textContent = err.message; return; }
  }
  await persistVars();
  let payload = collectRequest();
  const ok = await confirmProduction(`${payload.method} ${payload.path} → ${payload.base_url || ""}`);
  if (!ok) return;
  if (currentEnv() && currentEnv().production) payload = { ...payload, confirm_production: true };

  $("btn-send").disabled = true;
  $("response-meta").textContent = "Sending…";
  try {
    const data = await api("/api/request", { method: "POST", body: JSON.stringify(payload) });
    showResponse(data.response);
    await loadHistory();
    note("");
  } catch (err) {
    showResponse({ error: err.message, status: 0, status_text: "client error", duration_ms: 0, body: err.message });
  } finally {
    $("btn-send").disabled = false;
  }
}

function showResponse(resp) {
  state.lastResponse = resp;
  const status = resp.status || 0;
  const ok = status >= 200 && status < 400;
  $("response-meta").innerHTML = resp.error && !status
    ? `<span class="status bad">${escapeHtml(resp.error)}</span>`
    : `<span class="status ${ok ? "ok" : "bad"}">${status} ${escapeHtml(resp.status_text || "")}</span>
       <span>${resp.duration_ms || 0} ms</span>
       <span>${escapeHtml(resp.content_type || "")}</span>`;
  const schema = $("schema-note");
  const notes = $("schema-notes");
  if (resp.schema_ok == null) {
    schema.className = "chip hidden";
    schema.textContent = "";
    notes.textContent = "";
  } else if (resp.schema_ok) {
    schema.className = "chip ok";
    schema.textContent = "Matches schema";
    notes.textContent = "";
  } else {
    schema.className = "chip warn";
    schema.textContent = "Schema mismatch";
    notes.textContent = (resp.schema_notes || []).join(" · ");
  }
  state.lastRaw = resp.body || resp.error || "";
  renderResponseTab();
  $("btn-copy-response").disabled = !state.lastRaw;
}

function renderResponseTab() {
  const resp = state.lastResponse;
  const view = $("response-view");
  view.innerHTML = "";
  view.classList.add("text");
  if (!resp) { view.textContent = "Awaiting a request."; return; }
  if (state.responseTab === "headers") {
    view.textContent = JSON.stringify(resp.headers || {}, null, 2);
    return;
  }
  if (state.responseTab === "raw") {
    view.textContent = JSON.stringify(resp, null, 2);
    return;
  }
  const body = resp.body || resp.error || "";
  let parsed = null;
  if (body) {
    try { parsed = JSON.parse(body); } catch { parsed = null; }
  }
  const isJSON = parsed !== null && typeof parsed === "object";
  if (!isJSON) {
    view.textContent = body || "Empty response.";
    return;
  }
  view.classList.remove("text");
  view.appendChild(renderJSONNode("$", parsed, 0, null));
}

function renderJSONNode(path, value, depth, key) {
  const row = document.createElement("div");
  row.className = "json-row";
  const complex = value !== null && typeof value === "object";
  if (complex) {
    const isArr = Array.isArray(value);
    const entries = isArr ? value.map((v, i) => [String(i), v]) : Object.entries(value);
    const details = document.createElement("details");
    details.className = "json-node";
    if (depth < 2) details.open = true;
    const summary = document.createElement("summary");
    const label = isArr ? `[${entries.length}]` : `{${entries.length}}`;
    summary.innerHTML = `${key != null ? `<span class="jk">${escapeHtml(key)}</span> ` : ""}<span class="muted">${label}</span>`;
    details.appendChild(summary);
    const kids = document.createElement("div");
    kids.className = "json-children";
    const shown = entries.slice(0, 200);
    for (const [k, v] of shown) {
      const childPath = isArr ? `${path}[${k}]` : (path === "$" ? `$.${k}` : `${path}.${k}`);
      kids.appendChild(renderJSONNode(childPath, v, depth + 1, k));
    }
    if (entries.length > shown.length) {
      const more = document.createElement("div");
      more.className = "muted";
      more.textContent = `${entries.length - shown.length} more`;
      kids.appendChild(more);
    }
    details.appendChild(kids);
    row.appendChild(details);
  } else {
    const shown = typeof value === "string" ? JSON.stringify(value) : String(value);
    const line = document.createElement("span");
    line.innerHTML = `${key != null ? `<span class="jk">${escapeHtml(key)}</span> ` : ""}<span class="jv">${escapeHtml(shown)}</span>`;
    row.appendChild(line);
  }
  row.appendChild(jsonActions(value, key));
  return row;
}

function jsonActions(value, key) {
  const wrap = document.createElement("span");
  wrap.className = "json-actions";
  const copyBtn = document.createElement("button");
  copyBtn.type = "button";
  copyBtn.className = "mini";
  copyBtn.textContent = "Copy";
  copyBtn.addEventListener("click", async (e) => {
    e.preventDefault();
    e.stopPropagation();
    await copy(asVariable(value));
    note("Copied");
  });
  const useBtn = document.createElement("button");
  useBtn.type = "button";
  useBtn.className = "mini";
  useBtn.textContent = "Use";
  useBtn.addEventListener("click", async (e) => {
    e.preventDefault();
    e.stopPropagation();
    const suggestion = key == null ? "body" : (/^\d+$/.test(String(key)) ? "item" : String(key));
    const name = prompt("Variable name", suggestion);
    if (!name || !name.trim()) return;
    const varName = name.trim();
    await persistVars({ [varName]: asVariable(value) });
    renderVars();
    await copy(`{{${varName}}}`);
    note(`Saved ${varName}. Clipboard has {{${varName}}}`);
  });
  wrap.appendChild(copyBtn);
  wrap.appendChild(useBtn);
  return wrap;
}

function asVariable(value) {
  if (typeof value === "string") return value;
  if (value == null) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function renderVars() {
  const box = $("var-rows");
  const entries = Object.entries(state.variables || {});
  if (!entries.length) {
    box.innerHTML = `<p class="muted empty-vars">No variables yet.</p>`;
    return;
  }
  box.innerHTML = entries.map(([k, v]) => `
    <div class="var-row">
      <input data-var-name spellcheck="false" value="${escapeHtml(k)}" />
      <input data-var-value spellcheck="false" value="${escapeHtml(v)}" />
      <button type="button" class="ghost" data-var-del>Remove</button>
    </div>
  `).join("");
  box.querySelectorAll(".var-row").forEach((row) => {
    row.querySelector("[data-var-del]").addEventListener("click", async () => {
      row.remove();
      await persistVars();
      if (!$("var-rows").querySelector(".var-row")) renderVars();
    });
    row.querySelectorAll("input").forEach((input) => {
      input.addEventListener("change", () => persistVars());
    });
  });
}

function addVarRow() {
  const empty = $("var-rows").querySelector(".empty-vars");
  if (empty) empty.remove();
  const row = document.createElement("div");
  row.className = "var-row";
  row.innerHTML = `
    <input data-var-name spellcheck="false" placeholder="name" />
    <input data-var-value spellcheck="false" placeholder="value" />
    <button type="button" class="ghost" data-var-del>Remove</button>`;
  row.querySelector("[data-var-del]").addEventListener("click", async () => {
    row.remove();
    await persistVars();
    if (!$("var-rows").querySelector(".var-row")) renderVars();
  });
  row.querySelectorAll("input").forEach((input) => input.addEventListener("change", () => persistVars()));
  $("var-rows").appendChild(row);
  row.querySelector("[data-var-name]").focus();
}

async function persistVars(extra) {
  const m = {};
  document.querySelectorAll(".var-row").forEach((row) => {
    const name = row.querySelector("[data-var-name]").value.trim();
    if (!name) return;
    m[name] = row.querySelector("[data-var-value]").value;
  });
  if (extra) Object.assign(m, extra);
  state.variables = await api("/api/variables", { method: "PUT", body: JSON.stringify(m) });
}

async function loadVars() {
  state.variables = await api("/api/variables");
  renderVars();
}

function fillExample() {
  const ep = state.endpoint;
  if (!ep) return;
  (ep.parameters || []).forEach((p) => {
    if (p.example == null) return;
    const el = document.querySelector(`[data-param-in="${CSS.escape(p.in)}"][data-param-name="${CSS.escape(p.name)}"]`);
    if (el && !el.value) el.value = String(p.example);
  });
  if (ep.request_body && ep.request_body.example_json) {
    $("body-json").value = ep.request_body.example_json;
    setBodyMode("json");
    note("Filled from the spec example");
    return;
  }
  note("Filled parameter examples");
}

async function saveRequest() {
  if (!state.endpoint) return;
  const suggested = state.endpoint.operation_id || `${state.endpoint.method} ${state.endpoint.path}`;
  const name = prompt("Save request as", suggested);
  if (!name || !name.trim()) return;
  const p = collectRequest();
  try {
    await api("/api/saved", {
      method: "PUT",
      body: JSON.stringify({
        name: name.trim(),
        method: p.method,
        path: p.path,
        path_params: p.path_params,
        query: p.query,
        headers: p.headers,
        body: p.body,
        body_type: p.body_type,
        content_type: p.content_type,
        auth_mode: p.auth_mode,
      }),
    });
    await loadSaved();
    note(`Saved ${name.trim()}`);
  } catch (err) {
    note(err.message);
  }
}

async function copySnippet(kind) {
  if (!state.endpoint) return;
  try {
    await persistVars();
    const data = await api("/api/snippet", { method: "POST", body: JSON.stringify(collectRequest()) });
    await copy(data[kind] || "");
    note(kind === "curl" ? "Copied curl. The token is $JWT." : "Copied apismith call. The token is omitted.");
  } catch (err) {
    note(err.message);
  }
}

async function loadSaved() {
  state.saved = await api("/api/saved");
  renderSaved();
  fillSavedPick();
}

function renderSaved() {
  const nav = $("saved-list");
  if (!state.saved.length) {
    nav.innerHTML = `<p class="muted">No saved requests.</p>`;
    return;
  }
  nav.innerHTML = state.saved.map((item, i) => `
    <div class="saved-row">
      <button type="button" class="ep" data-saved-index="${i}">
        <span class="${methodClass(item.method)}">${escapeHtml(item.method)}</span>
        <span class="path">${escapeHtml(item.name)}</span>
      </button>
      <button type="button" class="ghost icon-btn" data-saved-del="${i}">×</button>
    </div>
  `).join("");
  nav.querySelectorAll("[data-saved-index]").forEach((btn) => {
    btn.addEventListener("click", () => applyCall(state.saved[Number(btn.dataset.savedIndex)]));
  });
  nav.querySelectorAll("[data-saved-del]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const item = state.saved[Number(btn.dataset.savedDel)];
      if (!item || !confirm(`Delete saved request “${item.name}”?`)) return;
      await api("/api/saved/" + encodeURIComponent(item.name), { method: "DELETE" });
      await loadSaved();
    });
  });
}

function fillSavedPick() {
  const sel = $("seq-saved-pick");
  if (!state.saved.length) {
    sel.innerHTML = `<option value="">No saved requests</option>`;
    return;
  }
  sel.innerHTML = state.saved.map((s) => `<option value="${escapeHtml(s.name)}">${escapeHtml(s.name)}</option>`).join("");
}

function syncSequence() {
  state.sequence.name = $("seq-name").value.trim();
  document.querySelectorAll(".seq-step").forEach((el, i) => {
    const step = state.sequence.steps[i];
    if (!step) return;
    step.expect = el.querySelector("[data-step-expect]").value.trim();
    const caps = {};
    el.querySelectorAll(".cap-row").forEach((row) => {
      const name = row.querySelector("[data-cap-name]").value.trim();
      const path = row.querySelector("[data-cap-path]").value.trim();
      if (name) caps[name] = path;
    });
    step.capture = caps;
  });
}

function renderSequence() {
  $("seq-name").value = state.sequence.name || "";
  const box = $("seq-steps");
  if (!state.sequence.steps.length) {
    box.innerHTML = `<p class="muted">No steps yet. Add the request that is open above, or add a saved request.</p>`;
    return;
  }
  box.innerHTML = state.sequence.steps.map((step, i) => {
    const caps = Object.entries(step.capture || {});
    return `
      <article class="seq-step ${i === state.selectedStep ? "selected" : ""}" data-step="${i}">
        <div class="top">
          <span class="${methodClass(step.method)}">${escapeHtml(step.method || "—")}</span>
          <code>${escapeHtml(step.path || step.saved || "")}</code>
          <input class="expect" data-step-expect placeholder="expect" value="${escapeHtml(step.expect || "")}" />
          <button type="button" class="ghost" data-step-load="${i}">Load</button>
          <button type="button" class="ghost" data-step-up="${i}">Up</button>
          <button type="button" class="ghost" data-step-down="${i}">Down</button>
          <button type="button" class="ghost" data-step-del="${i}">Remove</button>
        </div>
        <div class="captures">
          ${caps.map(([name, path]) => capRow(name, path)).join("")}
        </div>
        <button type="button" class="ghost" data-cap-add="${i}">Add capture</button>
      </article>`;
  }).join("");
  box.querySelectorAll("[data-step-load]").forEach((btn) => btn.addEventListener("click", () => loadStep(Number(btn.dataset.stepLoad))));
  box.querySelectorAll("[data-step-up]").forEach((btn) => btn.addEventListener("click", () => moveStep(Number(btn.dataset.stepUp), -1)));
  box.querySelectorAll("[data-step-down]").forEach((btn) => btn.addEventListener("click", () => moveStep(Number(btn.dataset.stepDown), 1)));
  box.querySelectorAll("[data-step-del]").forEach((btn) => btn.addEventListener("click", () => removeStep(Number(btn.dataset.stepDel))));
  box.querySelectorAll("[data-cap-add]").forEach((btn) => btn.addEventListener("click", () => addCaptureRow(btn.closest(".seq-step"))));
}

function capRow(name, path) {
  return `<div class="cap-row">
    <input data-cap-name placeholder="variable" spellcheck="false" value="${escapeHtml(name)}" />
    <input data-cap-path placeholder="$.id" spellcheck="false" value="${escapeHtml(path)}" />
    <button type="button" class="ghost" data-cap-del>Remove</button>
  </div>`;
}

function addCaptureRow(article) {
  const row = document.createElement("div");
  row.className = "cap-row";
  row.innerHTML = capRow("", "");
  const inner = row.firstElementChild;
  inner.querySelector("[data-cap-del]").addEventListener("click", () => inner.remove());
  article.querySelector(".captures").appendChild(inner);
  inner.querySelector("[data-cap-name]").focus();
}

function stepFromCurrent() {
  const p = collectRequest();
  state.stepSeq += 1;
  return {
    id: "step-" + state.stepSeq,
    method: p.method,
    path: p.path,
    path_params: p.path_params,
    query: p.query,
    headers: p.headers,
    body: p.body,
    body_type: p.body_type,
    content_type: p.content_type,
    auth_mode: p.auth_mode,
    expect: "",
    capture: {},
  };
}

function addCurrentStep() {
  if (!state.endpoint) { seqNote("Select an API first."); return; }
  syncSequence();
  state.sequence.steps.push(stepFromCurrent());
  state.selectedStep = state.sequence.steps.length - 1;
  renderSequence();
  seqNote("Added the current request.");
}

function addSavedStep() {
  const name = $("seq-saved-pick").value;
  const saved = state.saved.find((s) => s.name === name);
  if (!saved) { seqNote("Save a request first."); return; }
  syncSequence();
  state.stepSeq += 1;
  state.sequence.steps.push({
    id: "step-" + state.stepSeq,
    saved: saved.name,
    method: saved.method,
    path: saved.path,
    path_params: saved.path_params || {},
    query: saved.query || {},
    headers: saved.headers || {},
    body: saved.body || "",
    body_type: saved.body_type || "",
    content_type: saved.content_type || "",
    auth_mode: saved.auth_mode || "",
    expect: "",
    capture: {},
  });
  state.selectedStep = state.sequence.steps.length - 1;
  renderSequence();
  seqNote(`Added ${saved.name}.`);
}

function loadStep(index) {
  syncSequence();
  const step = state.sequence.steps[index];
  if (!step) return;
  state.selectedStep = index;
  applyCall(step);
  renderSequence();
  seqNote(`Loaded step ${index + 1} into the request form.`);
}

function updateSelectedStep() {
  if (!state.endpoint) { seqNote("Select an API first."); return; }
  syncSequence();
  if (state.selectedStep < 0 || !state.sequence.steps[state.selectedStep]) {
    seqNote("Load a step first, then update it.");
    return;
  }
  const prev = state.sequence.steps[state.selectedStep];
  const next = stepFromCurrent();
  next.id = prev.id;
  next.expect = prev.expect;
  next.capture = prev.capture;
  state.sequence.steps[state.selectedStep] = next;
  renderSequence();
  seqNote(`Updated step ${state.selectedStep + 1}.`);
}

function moveStep(index, delta) {
  syncSequence();
  const j = index + delta;
  if (j < 0 || j >= state.sequence.steps.length) return;
  const steps = state.sequence.steps;
  [steps[index], steps[j]] = [steps[j], steps[index]];
  if (state.selectedStep === index) state.selectedStep = j;
  else if (state.selectedStep === j) state.selectedStep = index;
  renderSequence();
}

function removeStep(index) {
  syncSequence();
  state.sequence.steps.splice(index, 1);
  if (state.selectedStep === index) state.selectedStep = -1;
  else if (state.selectedStep > index) state.selectedStep -= 1;
  renderSequence();
}

function newSequence() {
  state.sequence = { name: "", steps: [] };
  state.selectedStep = -1;
  renderSequence();
  $("seq-results").innerHTML = "";
  seqNote("New sequence.");
}

async function saveSequence() {
  syncSequence();
  if (!state.sequence.name) { seqNote("Name the sequence."); return; }
  if (!state.sequence.steps.length) { seqNote("Add a step first."); return; }
  try {
    state.sequence = await api("/api/sequences", { method: "PUT", body: JSON.stringify(state.sequence) });
    await loadSequences();
    seqNote(`Saved ${state.sequence.name}.`);
  } catch (err) {
    seqNote(err.message);
  }
}

async function deleteSequence() {
  syncSequence();
  const name = state.sequence.name;
  if (!name) { seqNote("This sequence is not saved."); return; }
  if (!confirm(`Delete sequence “${name}”?`)) return;
  try {
    await api("/api/sequences/" + encodeURIComponent(name), { method: "DELETE" });
    newSequence();
    await loadSequences();
    seqNote(`Deleted ${name}.`);
  } catch (err) {
    seqNote(err.message);
  }
}

async function runSequence() {
  syncSequence();
  if (!state.sequence.steps.length) { seqNote("Add a step first."); return; }
  await persistVars();
  const detail = `${state.sequence.steps.length} steps → ${$("base-url").value.trim()}`;
  const ok = await confirmProduction(detail);
  if (!ok) return;
  const payload = {
    sequence: state.sequence,
    environment: state.envId,
    base_url: $("base-url").value.trim(),
    auth_mode: selectedAuthMode(),
    jwt: $("jwt").value.trim(),
    custom_authorization: $("custom-auth").value.trim(),
    confirm_production: !!(currentEnv() && currentEnv().production),
  };
  $("btn-seq-run").disabled = true;
  seqNote("Running…");
  try {
    const result = await api("/api/run", { method: "POST", body: JSON.stringify(payload) });
    state.variables = result.variables || {};
    renderVars();
    renderSeqResults(result);
    await loadHistory();
    seqNote(result.ok ? "Sequence passed." : "Sequence stopped on a failed step.");
  } catch (err) {
    seqNote(err.message);
  } finally {
    $("btn-seq-run").disabled = false;
  }
}

function renderSeqResults(result) {
  const box = $("seq-results");
  const steps = (result && result.steps) || [];
  if (!steps.length) { box.innerHTML = ""; return; }
  box.innerHTML = steps.map((s, i) => `
    <button type="button" class="seq-result ${s.skipped ? "" : s.ok ? "ok" : "bad"}" data-result="${i}">
      <div class="row">
        <span>${s.skipped ? "–" : s.ok ? "✓" : "✗"}</span>
        <span class="${methodClass(s.method)}">${escapeHtml(s.method || "")}</span>
        <span>${escapeHtml(s.path || "")}</span>
        <span class="meta">${s.skipped ? "skipped" : (s.status || "—")} · ${s.duration_ms || 0}ms</span>
      </div>
      ${s.error ? `<div class="error-text">${escapeHtml(s.error)}</div>` : ""}
      ${(s.captured ? Object.entries(s.captured) : []).map(([k, v]) => `<div class="meta">${escapeHtml(k)} = ${escapeHtml(v)}</div>`).join("")}
    </button>
  `).join("");
  box.querySelectorAll("[data-result]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const step = steps[Number(btn.dataset.result)];
      if (!step || step.skipped) return;
      showResponse({
        status: step.status,
        status_text: step.status_text || "",
        duration_ms: step.duration_ms,
        body: step.body || "",
        error: step.error || "",
        headers: {},
      });
    });
  });
}

async function loadSequences() {
  state.sequences = await api("/api/sequences");
  renderSequenceList();
}

function renderSequenceList() {
  const nav = $("sequence-list");
  if (!state.sequences.length) {
    nav.innerHTML = `<p class="muted">No sequences.</p>`;
    return;
  }
  nav.innerHTML = state.sequences.map((seq, i) => `
    <div class="seq-row">
      <button type="button" class="ep" data-seq-index="${i}">
        <span class="path">${escapeHtml(seq.name)}</span>
        <span class="sum">${(seq.steps || []).length}</span>
      </button>
    </div>
  `).join("");
  nav.querySelectorAll("[data-seq-index]").forEach((btn) => {
    btn.addEventListener("click", () => openSequence(state.sequences[Number(btn.dataset.seqIndex)]));
  });
}

function openSequence(seq) {
  state.sequence = {
    name: seq.name,
    steps: (seq.steps || []).map((step) => ({ ...step, capture: { ...(step.capture || {}) }, path_params: { ...(step.path_params || {}) }, query: { ...(step.query || {}) }, headers: { ...(step.headers || {}) } })),
  };
  state.selectedStep = -1;
  state.stepSeq = Math.max(state.stepSeq, state.sequence.steps.length);
  renderSequence();
  $("seq-results").innerHTML = "";
  seqNote(`Opened ${seq.name}.`);
}

async function generateJWT() {
  $("btn-generate").disabled = true;
  try {
    const data = await api("/api/auth/jwt", {
      method: "POST",
      body: JSON.stringify({
        environment: state.envId,
        client_id: $("client-id").value.trim(),
        username: $("username").value.trim(),
        password: $("password").value,
      }),
    });
    $("jwt").value = data.access_token || "";
    setAuthStatus(true, data.status);
  } catch (err) {
    setAuthStatus(false, null, err.message);
  } finally {
    $("btn-generate").disabled = false;
  }
}

function setAuthStatus(ok, status, err) {
  const el = $("auth-status");
  if (err) {
    el.className = "status bad";
    el.textContent = err;
    return;
  }
  if (ok || (status && status.authenticated)) {
    el.className = "status ok";
    el.textContent = "Authenticated";
  } else {
    el.className = "status muted";
    el.textContent = "Not authenticated";
  }
}

async function loadHistory() {
  const items = await api("/api/history");
  const box = $("history-list");
  if (!items.length) {
    box.className = "history-list muted";
    box.textContent = "No requests yet.";
    return;
  }
  box.className = "history-list";
  box.innerHTML = items.map((it) => `
    <button type="button" class="hist" data-id="${it.id}">
      <div class="row">
        <span>${it.ok ? "✓" : "✗"}</span>
        <span class="${methodClass(it.method)}">${escapeHtml(it.method)}</span>
        <span>${escapeHtml(it.path)}</span>
      </div>
      <div class="meta">${it.status || "—"} · ${it.duration_ms}ms · ${escapeHtml(it.environment || "")}</div>
    </button>
  `).join("");
  box.querySelectorAll(".hist").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const item = await api(`/api/history/${btn.dataset.id}`);
      applyCall(item);
    });
  });
}

function bindUI() {
  $("environment").addEventListener("change", (e) => {
    state.envId = e.target.value;
    applyEnv();
  });
  $("api-search").addEventListener("input", renderEndpointList);
  document.querySelectorAll("[data-side]").forEach((btn) => {
    btn.addEventListener("click", () => setSide(btn.dataset.side));
  });
  document.querySelectorAll("input[name=auth-mode]").forEach((el) => {
    el.addEventListener("change", () => setAuthMode(selectedAuthMode()));
  });
  document.querySelectorAll("[data-body-mode]").forEach((b) => {
    b.addEventListener("click", () => {
      const next = b.dataset.bodyMode;
      if (state.bodyMode === "form" && next === "json") {
        const fromForm = bodyFromForm();
        if (fromForm) $("body-json").value = fromForm;
      }
      if (state.bodyMode === "json" && next === "text" && !$("body-text").value.trim()) {
        $("body-text").value = $("body-json").value;
      }
      setBodyMode(next);
    });
  });
  document.querySelectorAll("[data-tab]").forEach((b) => {
    b.addEventListener("click", () => {
      state.responseTab = b.dataset.tab;
      document.querySelectorAll("[data-tab]").forEach((x) => x.classList.toggle("active", x === b));
      renderResponseTab();
    });
  });
  $("btn-generate").addEventListener("click", generateJWT);
  $("btn-copy-jwt").addEventListener("click", () => copy($("jwt").value));
  $("btn-send").addEventListener("click", sendRequest);
  $("btn-save-request").addEventListener("click", saveRequest);
  $("btn-fill-example").addEventListener("click", fillExample);
  $("btn-copy-curl").addEventListener("click", () => copySnippet("curl"));
  $("btn-copy-call").addEventListener("click", () => copySnippet("call"));
  $("btn-add-var").addEventListener("click", addVarRow);
  $("btn-add-header").addEventListener("click", () => addHeaderRow());
  $("body-file").addEventListener("change", () => {
    const file = $("body-file").files && $("body-file").files[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      state.fileBody = String(reader.result || "");
      state.fileName = file.name;
      state.fileType = typeFromName(file.name);
      $("file-name").textContent = `${file.name} · ${state.fileBody.length} characters · ${state.fileType}`;
    };
    reader.readAsText(file);
  });
  $("btn-format-json").addEventListener("click", () => {
    try {
      $("body-json").value = JSON.stringify(JSON.parse($("body-json").value || "{}"), null, 2);
      $("json-error").textContent = "";
    } catch (err) {
      $("json-error").textContent = err.message;
    }
  });
  $("body-json").addEventListener("input", () => {
    if (!$("body-json").value.trim()) { $("json-error").textContent = ""; return; }
    try { JSON.parse($("body-json").value); $("json-error").textContent = ""; }
    catch (err) { $("json-error").textContent = err.message; }
  });
  $("btn-copy-response").addEventListener("click", () => copy(state.lastRaw));
  $("btn-clear-history").addEventListener("click", async () => {
    await api("/api/history/clear", { method: "POST" });
    await loadHistory();
  });
  $("btn-seq-add").addEventListener("click", addCurrentStep);
  $("btn-seq-add-saved").addEventListener("click", addSavedStep);
  $("btn-seq-update").addEventListener("click", updateSelectedStep);
  $("btn-seq-new").addEventListener("click", newSequence);
  $("btn-seq-save").addEventListener("click", saveSequence);
  $("btn-seq-delete").addEventListener("click", deleteSequence);
  $("btn-seq-run").addEventListener("click", runSequence);
  $("seq-steps").addEventListener("click", (e) => {
    const del = e.target.closest("[data-cap-del]");
    if (del) del.closest(".cap-row").remove();
  });
}

async function boot() {
  bindUI();
  renderVars();
  renderSequence();
  fillSavedPick();
  const [health, envs, catalog] = await Promise.all([
    api("/api/health"),
    api("/api/environments"),
    api("/api/catalog"),
  ]);
  $("spec-title").textContent = `${health.title} · ${health.endpoints} endpoints`;
  renderEnvironments(envs);
  renderCatalog(catalog);
  try {
    const status = await api("/api/auth/status");
    setAuthStatus(status.authenticated, status);
  } catch { /* ignore */ }
  await Promise.all([loadHistory(), loadSaved(), loadSequences(), loadVars()]);
}

boot().catch((err) => {
  $("spec-title").textContent = "failed to load: " + err.message;
});
