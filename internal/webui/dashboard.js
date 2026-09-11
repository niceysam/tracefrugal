/* Shared by the offline Go app and the public demo. No dependencies or telemetry. */
"use strict";
const boot = JSON.parse(document.getElementById("bootstrap").textContent);
const $ = id => document.getElementById(id);
const esc = value => String(value ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const num = value => Number(value).toLocaleString("en-US");
const money = value => value == null ? "—" : "$" + Number(value).toFixed(4);
const finite = value => typeof value === "number" && Number.isFinite(value) && value >= 0;
const pct = value => value == null ? "—" : (value > 0 ? "+" : "") + value.toFixed(1) + "%";
const colors = ["#4269de", "#15936c", "#d9a13b", "#9060c7", "#8da1c4"];
let state, busy = false, view = "overview", imported = false;

function announce(text, error = false) {
  $("notice").textContent = text;
  $("notice").classList.toggle("error", error);
}
function showView(name) {
  if (!["overview", "history", "connect"].includes(name)) return;
  view = name;
  document.querySelectorAll(".view").forEach(el => el.hidden = el.id !== name);
  document.querySelectorAll(".sidebar [data-view]").forEach(el => {
    if (el.dataset.view === name) el.setAttribute("aria-current", "page");
    else el.removeAttribute("aria-current");
  });
}
function metric(label, value, note, good = false) {
  return `<article class="metric"><span class="metric-label">${esc(label)}</span><strong class="${good ? "good" : ""}">${esc(value)}</strong><small>${esc(note)}</small></article>`;
}
function tokenChart(report) {
  if (!report) return '<div class="empty">No recorded usage for the active settings yet.</div>';
  const values = [report.tokens.input, report.tokens.cached_input, report.tokens.cache_write, report.tokens.cache_write_1h, report.tokens.output];
  const names = ["New input", "Cached input", "Cache writes · 5 min", "Cache writes · 1 hour", "Output"];
  const total = values.reduce((a, b) => a + b, 0);
  let offset = 0;
  const arcs = values.map((v, i) => {
    const length = total ? v / total * 100 : 0;
    const arc = `<circle cx="60" cy="60" r="46" fill="none" stroke="${colors[i]}" stroke-width="14" pathLength="100" stroke-dasharray="${length} ${100 - length}" stroke-dashoffset="${-offset}"/>`;
    offset += length;
    return arc;
  }).join("");
  return `<div class="donut-wrap"><svg class="donut" viewBox="0 0 120 120" role="img" aria-label="${esc(num(total))} total tokens; category values follow"><circle cx="60" cy="60" r="46" fill="none" stroke="#edf1f5" stroke-width="14"/>${arcs}</svg><div class="token-total"><strong>${esc(num(total))}</strong><small>total tokens</small></div></div>` +
    values.map((v, i) => v || i === 0 || i === 1 || i === 4 ? `<div class="token-row"><span><i class="dot" style="background:${colors[i]}"></i>${names[i]}</span><strong>${num(v)}</strong></div>` : "").join("") +
    '<p class="token-note">Billing categories, not context-window size. Usage totals cannot separate chat history from tool schemas.</p>';
}
function historyPoints(entries) {
  const chronological = entries.slice().reverse();
  const costs = new Map();
  chronological.forEach(e => {
    if (finite(e.baseline_cost_usd)) costs.set(e.before_sha256, e.baseline_cost_usd);
    if (finite(e.candidate_cost_usd)) costs.set(e.after_sha256, e.candidate_cost_usd);
  });
  const points = [];
  let trial = 0;
  chronological.forEach(e => {
    if (e.action === "experiment" && finite(e.baseline_cost_usd)) {
      trial++;
      if (!points.length) points.push({label:"Start", cost:e.baseline_cost_usd, status:"baseline", caption:"First baseline"});
      // Plot each measured baseline separately; task mixes and prices can change.
      if (trial > 1) points.push({label:`${trial} before`, cost:e.baseline_cost_usd, status:"baseline", caption:`Trial ${trial} baseline`});
      if (finite(e.candidate_cost_usd)) points.push({label:`${trial} after`, cost:e.candidate_cost_usd, status:e.status, caption:`Trial ${trial} · ${e.status}`});
    } else if (e.status === "rolled_back" && costs.has(e.after_sha256)) {
      points.push({label:"Restored", cost:costs.get(e.after_sha256), status:"rolled_back", caption:"Previous evaluation of restored settings"});
    }
  });
  return points;
}
function renderTrend() {
  const usage = !!state.report;
  const all = usage ? state.report.tasks.map(t => ({label:t.task_id, cost:t.estimated_cost_usd, status:"baseline", caption:t.task_id})) : historyPoints(state.entries);
  const points = all.slice(-8);
  $("trend-title").textContent = usage ? "Cost by task" : "Cost over experiments";
  $("trend-caption").textContent = usage ? "Estimated token spend, including failed attempts." : "Baseline and trial costs. Dots marked rejected were not applied." + (all.length > 8 ? " Showing the latest 8 points." : "");
  $("trend-legend").hidden = usage;
  document.querySelector(".unit").textContent = usage ? "USD / task" : "USD / evaluation";
  if (!points.length) {
    $("trend").innerHTML = '<div class="empty">Your first recorded evaluation will appear here.</div>';
    $("chart-table").innerHTML = "";
    return;
  }
  const max = Math.max(...points.map(p => p.cost), 0.000001) * 1.28;
  const width = Math.max(320, $("trend").clientWidth || 610);
  const x = i => points.length === 1 ? (width+36)/2 : 66 + i * ((width-100) / (points.length - 1));
  const y = n => 215 - n / max * 175;
  let svg = `<svg viewBox="0 0 ${width} 260" role="img" aria-labelledby="plot-title"><title id="plot-title">` + esc(usage ? "Estimated cost by task. Values are also in the chart table." : "Estimated evaluation cost over trials. Rejected points are excluded from the applied line.") + '</title>';
  for (let i = 0; i < 4; i++) {
    const v = max * i / 3;
    svg += `<line class="grid" x1="60" x2="${width-15}" y1="${y(v)}" y2="${y(v)}"/><text x="51" y="${y(v) + 4}" text-anchor="end">${money(v)}</text>`;
  }
  const accepted = p => ["baseline", "applied", "rolled_back"].includes(p.status);
  if (!usage) {
    const active = points.map((p, i) => accepted(p) ? `${x(i)},${y(p.cost)}` : null).filter(Boolean);
    svg += `<polyline class="line" points="${active.join(" ")}"/>`;
  }
  points.forEach((p, i) => {
    const color = p.status === "rolled_back" ? "#4269de" : accepted(p) ? "#157755" : "#b94242";
    const label = p.label.length > 12 ? p.label.slice(0, 10) + "…" : p.label;
    if (usage) svg += `<rect x="${x(i)-14}" y="${y(p.cost)}" width="28" height="${215-y(p.cost)}" rx="4" fill="#4269de"/>`;
    else svg += `<circle class="point" cx="${x(i)}" cy="${y(p.cost)}" r="6" fill="${color}"><title>${esc(p.caption)}: ${money(p.cost)}</title></circle>`;
    const showLabel = points.length <= 4 || width > 480 || i % 2 === 0 || i === points.length-1;
    if (showLabel) svg += `<text class="point-label" x="${x(i)}" y="${y(p.cost)-14}" text-anchor="middle">${money(p.cost)}</text><text x="${x(i)}" y="242" text-anchor="middle">${esc(label)}</text>`;
    if (!usage && !accepted(p)) svg += `<text x="${x(i)}" y="${y(p.cost)+20}" text-anchor="middle" style="fill:#b94242">${esc(p.status)}</text>`;
  });
  $("trend").innerHTML = svg + "</svg>";
  $("chart-table").innerHTML = `<table><thead><tr><th>Point</th><th>Estimated cost</th><th>Status</th></tr></thead><tbody>${all.map(p => `<tr><td>${esc(p.caption)}</td><td>${money(p.cost)}</td><td>${esc(p.status)}</td></tr>`).join("")}</tbody></table>`;
}
const statusTitle = {applied:"Lower-cost settings applied",rejected:"Trial rejected · settings kept",rolled_back:"Previous settings restored",resumed:"Experiments allowed again",error:"Trial could not finish",running:"Evaluation in progress",applying:"Applying candidate",recommended:"Lower-cost candidate found",unchanged:"No settings change"};
function eventRow(e, full = false) {
  const cap = e.before_output_cap != null && e.after_output_cap != null ? `Output limit: ${num(e.before_output_cap)} → ${num(e.after_output_cap)} tokens` : e.action === "resume" ? "Pause removed. This does not start a scheduler." : e.message;
  const when = new Date(e.started_at).toLocaleString("en-US", {month:"short", day:"numeric", hour:"2-digit", minute:"2-digit"});
  const cost = finite(e.candidate_cost_usd) ? money(e.candidate_cost_usd) : e.status === "rolled_back" ? "Restored" : "—";
  const change = e.cost_per_success_change_percent;
  let row = `<div class="history-row"><span class="event-icon ${esc(e.status)}" aria-hidden="true">${e.status === "applied" ? "✓" : e.status === "rejected" || e.status === "error" ? "×" : "↺"}</span><div class="event-body"><strong>${esc(statusTitle[e.status] || e.status)}</strong><p>${esc(cap)}</p></div><span class="event-badge">${esc(when)}</span><div class="event-cost">${esc(cost)}<small>${change == null ? "Recorded event" : esc(pct(change)) + " / success"}</small></div></div>`;
  if (full) {
    const link = !state.demo && e.has_report && /^\d{8}T\d{6}\.\d{9}Z$/.test(e.id) ? `<p><a href="runs/${encodeURIComponent(e.id)}" target="_blank" rel="noreferrer">Open the task-by-task report ↗</a></p>` : "";
    row = `<article class="history-detail">${row}<p>${esc(e.message)}</p>${link}<details><summary>Record details</summary><pre>${esc(JSON.stringify(e, null, 2))}</pre></details></article>`;
  }
  return row;
}
function renderAction() {
  let title, text, buttons;
  if (state.report) {
    title = "Ready to test a lower-cost setup?";
    text = "A report shows usage. To apply settings safely, connect an evaluator that checks your tasks.";
    buttons = '<button class="primary" data-view="connect">Set up optimization →</button>';
  } else if (state.paused) {
    title = "Previous settings restored. Experiments are paused.";
    text = "You are back in control. Allow experiments again whenever you are ready.";
    buttons = '<button class="primary" data-action="resume">Allow experiments again</button>';
  } else if (state.demo) {
    title = "Try a smaller response limit.";
    text = "We test the same two sample tasks. A cheaper result is kept only if both still pass. All numbers here are simulated.";
    buttons = '<button class="primary" data-action="try">Try optimization →</button>';
    if (state.rollback_id) buttons += '<button class="secondary" data-action="undo">Undo last change</button>';
  } else {
    const last = state.entries.find(e => e.action === "experiment");
    title = last?.status === "rejected" ? "The latest trial was rejected. Your settings stayed put." : "Review the result. Keep control of your settings.";
    text = "Results refresh automatically. Hourly trials run only while your configured experiment scheduler is running.";
    buttons = state.rollback_id ? '<button class="primary" data-action="undo">Undo last change</button>' : '<button class="primary" data-view="connect">Set up experiments →</button>';
  }
  $("next-action").innerHTML = `<div class="action-copy"><span class="action-icon" aria-hidden="true">↘</span><div><h2>${esc(title)}</h2><p>${esc(text)}</p></div></div><div class="action-buttons">${buttons}</div>`;
  $("quick-action").innerHTML = buttons.slice(0, buttons.indexOf("</button>") + 9);
}
function render() {
  const report = state.report || state.current_report;
  const liveDemo = state.synthetic && !state.demo;
  $("source-badge").textContent = state.demo ? "Sample workspace" : imported ? "Your report · browser only" : liveDemo ? "Local synthetic demo" : "Local data";
  $("source-badge").classList.toggle("demo", !!(state.demo || state.synthetic));
  $("demo-banner").hidden = !state.demo;
  $("subtitle").textContent = state.demo ? "Try a change below. Watch the graph. Undo it whenever you want." : state.report ? "Recorded usage from your report. Costs use its supplied prices." : "Your recorded evaluations. Updated every 3 seconds.";
  $("token-caption").textContent = state.report ? "All recorded requests in this report" : "Latest evaluation of the active settings";
  const current = state.entries.find(e => e.applied && e.action === "experiment" && e.after_sha256 === state.active_hash);
  const savings = current?.cost_per_success_change_percent;
  const quality = report ? `${report.successful_tasks} / ${report.successful_tasks + report.failed_tasks + report.tasks_without_result}` : "—";
  const input = report ? report.tokens.input + report.tokens.cached_input + report.tokens.cache_write + report.tokens.cache_write_1h : 0;
  const hit = input ? report.tokens.cached_input / input * 100 : null;
  $("metrics").innerHTML = metric(state.report ? "Recorded token cost" : "Cost of active settings", report ? money(report.estimated_cost_usd) : "—", state.report ? "USD estimate · not an invoice" : "USD · latest matching evaluation") +
    (state.report ? metric("Input served from cache", hit == null ? "—" : hit.toFixed(1) + "%", "Share of recorded input tokens", true) : metric("Applied improvement", savings == null ? "—" : pct(savings), "Cost / success vs that trial's baseline", savings < 0)) +
    metric("Tasks passing checks", quality, report?.tasks_without_result ? `${report.tasks_without_result} results missing` : "Your evaluator defines a pass", report && !report.failed_tasks && !report.tasks_without_result) +
    (state.report ? metric("Recorded requests", num(report.requests), "Each request counted once") : metric("Total testing cost", money(state.spend), "Baseline + candidate evaluations"));
  $("tokens").innerHTML = tokenChart(report);
  renderTrend();
  renderAction();
  const empty = '<div class="panel empty">No changes recorded here. Connect an evaluator to start building your history.</div>';
  $("recent").innerHTML = state.entries.slice(0, 3).map(e => eventRow(e)).join("") || empty;
  $("history-list").innerHTML = state.entries.map(e => eventRow(e, true)).join("") || empty;
  $("history-count").textContent = state.entries.length;
  $("history-summary").innerHTML = `<p class="history-summary">${state.entries.length} recorded events · ${state.paused ? "Experiments paused" : "Experiments allowed"}${state.demo ? " · Demo resets when you reload this page." : " · Scheduling is controlled by the local experiment runner."}</p>`;
  $("scope-note").textContent = state.demo || state.synthetic
    ? "Synthetic demonstration. No model calls or actual savings. Quality is simulated with two tasks. Rejected trial costs still count toward testing spend."
    : state.report
      ? "USD estimates from the report's price book, not your provider invoice. Tool fees and infrastructure are excluded. " + (imported ? "This file was read only in this browser; it was not uploaded." : "Usage records stay on your machine.")
      : "These are evaluation costs, not live application spend. Settings apply only when your application reads the managed profile. Undo cannot refund calls or reverse actions already performed. Missing usage from failed calls may be absent.";
  showView(view);
}

// Public demo uses the Go demo's exact two-task fixture and explicit synthetic rates.
function demoReport(cap) {
  const cost = (500 * 5 + 2000 * 0.5 + cap * 15) / 1e6;
  const success = cap >= 256;
  return {schema_version:1, price_label:"SYNTHETIC demo rates — no model calls", requests:2,
    tokens:{input:1000,cached_input:4000,cache_write:0,cache_write_1h:0,output:cap*2},
    estimated_cost_usd:cost*2, successful_tasks:success?2:0, failed_tasks:success?0:2,tasks_without_result:0,
    estimated_cost_per_success_usd:success?cost:null,
    tasks:["explain-error","extract-fields"].map(task_id => ({task_id,requests:1,estimated_cost_usd:cost,success}))};
}
function demoTrial() {
  const before = state.active_cap, after = Math.max(64, Math.floor(before / 2));
  const a = demoReport(before), b = demoReport(after), applied = b.successful_tasks === 2 && after < before;
  const e = {id:"demo-"+(state.entries.length+1),started_at:new Date().toISOString(),action:"experiment",
    status:applied?"applied":"rejected",applied, before_output_cap:before,after_output_cap:after,
    before_sha256:"demo-"+before,after_sha256:"demo-"+after,baseline_cost_usd:a.estimated_cost_usd,candidate_cost_usd:b.estimated_cost_usd,
    cost_per_success_change_percent:applied?(b.estimated_cost_usd/a.estimated_cost_usd-1)*100:null,
    message:applied?"Both sample tasks passed. The lower-cost response limit was applied.":"Both sample tasks failed at this response limit. The candidate was rejected; the active settings were kept."};
  state.entries.unshift(e);
  state.spend += a.estimated_cost_usd + b.estimated_cost_usd;
  if (applied) { state.active_cap = after; state.active_hash = "demo-"+after; state.rollback_id = e.id; }
  state.current_report = demoReport(state.active_cap);
  return applied;
}
function resetDemo() {
  imported = false;
  state = {demo:true,synthetic:true,entries:[],active_cap:1024,active_hash:"demo-1024",paused:false,spend:0};
  demoTrial();
  announce("");
  render();
}
async function refresh() {
  if (boot.mode === "demo" || imported) return;
  try {
    const response = await fetch("api/state", {cache:"no-store"});
    if (!response.ok) throw new Error(await response.text());
    const fresh = await response.json();
    if (imported) return;
    // Preserve keyboard focus, expanded details, and chart state between polls.
    if (state && JSON.stringify(fresh) === JSON.stringify(state)) {
      if ($("notice").classList.contains("error")) announce("Connection restored. Showing the latest recorded data.");
      return;
    }
    state = fresh;
    render();
    if ($("notice").classList.contains("error")) announce("Connection restored. Showing the latest recorded data.");
  } catch (error) {
    announce("Data unavailable. " + error.message + " Any visible values are from the last successful update.", true);
    if (!state) {
      state = {entries:[],spend:0};
      render();
    }
  }
}
async function localAction(action) {
  const data = new URLSearchParams({token:boot.token || ""});
  if (action === "rollback") data.set("id", state.rollback_id);
  const response = await fetch(action, {method:"POST",body:data,redirect:"follow"});
  if (!response.ok) throw new Error(await response.text());
  await refresh();
}
async function act(action) {
  if (busy) return;
  if (action === "undo") {
    const e = state.entries.find(e => e.id === state.rollback_id);
    if (!e) return;
    $("undo-description").textContent = e.before_output_cap != null ? `The response limit returns from ${num(e.after_output_cap)} to ${num(e.before_output_cap)} tokens.` : "The managed profile returns to the snapshot before this experiment.";
    $("undo-dialog").showModal();
    return;
  }
  busy = true;
  document.querySelectorAll("[data-action]").forEach(b => b.disabled = true);
  try {
    if (action === "try" && state.demo && !state.paused) {
      announce("Testing a smaller response limit against the two sample tasks…");
      await new Promise(resolve => setTimeout(resolve, 650));
      const applied = demoTrial();
      announce(applied ? "Applied. Both sample tasks still pass, and the estimated cost fell. You can undo this change below." : "Rejected. The cheaper candidate failed both sample tasks. Your active settings were kept.", !applied);
    } else if (action === "resume") {
      if (state.demo) {
        state.paused = false;
        state.entries.unshift({id:"demo-"+(state.entries.length+1),started_at:new Date().toISOString(),action:"resume",status:"resumed",message:"Experiments allowed again. No model calls were made."});
      } else await localAction("resume");
      announce("Experiments are allowed again. A stopped scheduler is not automatically restarted.");
    } else if (action === "rollback") {
      if (state.demo) {
        const source = state.entries.find(e => e.id === state.rollback_id);
        if (!source || state.active_hash !== source.after_sha256) throw new Error("Settings changed. Please review the latest history.");
        state.entries.unshift({id:"demo-"+(state.entries.length+1),started_at:new Date().toISOString(),action:"rollback",status:"rolled_back",applied:true,
          before_output_cap:state.active_cap,after_output_cap:source.before_output_cap,before_sha256:state.active_hash,after_sha256:source.before_sha256,message:"Previous settings restored. Future experiments paused."});
        state.active_cap = source.before_output_cap;
        state.active_hash = source.before_sha256;
        state.current_report = demoReport(state.active_cap);
        state.paused = true;
        state.rollback_id = state.entries.find(e => e.action === "experiment" && e.applied && e.after_sha256 === state.active_hash)?.id;
      } else await localAction("rollback");
      announce("Restored the previous settings and paused experiments. Your full history is still available.");
    }
  } catch (error) { announce(error.message, true); }
  finally { busy = false; render(); }
}
function validateReport(r) {
  const integer = v => Number.isSafeInteger(v) && v >= 0;
  if (!r || r.schema_version !== 1 || typeof r.price_label !== "string" || !Array.isArray(r.tasks) || !r.tokens ||
    !["input","cached_input","cache_write","cache_write_1h","output"].every(k => integer(r.tokens[k])) ||
    !["requests","successful_tasks","failed_tasks","tasks_without_result"].every(k => integer(r[k])) ||
    !finite(r.estimated_cost_usd) || !(r.estimated_cost_per_success_usd === null || finite(r.estimated_cost_per_success_usd))) throw new Error("Choose a TraceFrugal JSON report exported with --format json.");
  const ids = new Set();
  let requests=0,cost=0,passed=0,failed=0,unknown=0;
  for (const t of r.tasks) {
    if (!t || typeof t.task_id !== "string" || !t.task_id || ids.has(t.task_id) || !integer(t.requests) || !finite(t.estimated_cost_usd) || ![true,false,null].includes(t.success)) throw new Error("The report contains an invalid or duplicate task.");
    ids.add(t.task_id); requests+=t.requests; cost+=t.estimated_cost_usd;
    if(t.success===true) passed++; else if(t.success===false) failed++; else unknown++;
  }
  if (requests !== r.requests || passed !== r.successful_tasks || failed !== r.failed_tasks || unknown !== r.tasks_without_result || Math.abs(cost-r.estimated_cost_usd)>1e-8*Math.max(1,cost)) throw new Error("The report totals do not match its task records.");
  const perSuccess = passed && !unknown ? cost / passed : null;
  if ((perSuccess === null) !== (r.estimated_cost_per_success_usd === null) || (perSuccess !== null && Math.abs(perSuccess-r.estimated_cost_per_success_usd)>1e-8*Math.max(1,perSuccess))) throw new Error("The cost per successful task does not match the report.");
  return r;
}
const guides = {
  openai:"Supported: final Responses API usage. Add the usage recorder to your application and supply your model prices. This does not connect to the ChatGPT subscription interface.",
  anthropic:"Supported: final Messages API usage, including separate cache reads and cache writes. Add the usage recorder to your application. This does not attach to Claude Desktop or Claude Code sessions.",
  other:"Other providers can write the normalized TraceFrugal JSONL format. Native Codex / Claude Code sessions and subscription billing are not automatically imported."
};
function provider(name) {
  $("provider-guide").textContent = guides[name];
  document.querySelectorAll("[data-provider]").forEach(b => b.setAttribute("aria-pressed", String(b.dataset.provider === name)));
}
document.addEventListener("click", event => {
  const nav = event.target.closest("[data-view]");
  if (nav) showView(nav.dataset.view);
  const button = event.target.closest("[data-action]");
  if (button) act(button.dataset.action);
  const source = event.target.closest("[data-provider]");
  if (source) provider(source.dataset.provider);
});
$("reset").addEventListener("click", () => { if(!busy) resetDemo(); });
$("cancel-undo").addEventListener("click", () => $("undo-dialog").close());
$("confirm-undo").addEventListener("click", () => { $("undo-dialog").close(); act("rollback"); });
$("report-file").addEventListener("change", async event => {
  const file = event.target.files[0];
  if (!file) return;
  try {
    if (file.size > 4*1024*1024) throw new Error("Choose a report smaller than 4 MB.");
    const report = validateReport(JSON.parse(await file.text()));
    imported = true;
    state = {report,entries:[],spend:0};
    showView("overview");
    render();
    announce("Opened " + file.name + " in your browser. No data was uploaded. Reload this page to return to your previous workspace.");
    $("import-message").textContent = "";
  } catch (error) { $("import-message").textContent = "Could not open this file. " + error.message; }
});
provider("openai");
if (typeof window !== "undefined") window.addEventListener("resize", () => { if (state) renderTrend(); });
if (boot.mode === "demo") resetDemo();
else { refresh(); setInterval(() => { if (!busy && !$("undo-dialog").open) refresh(); }, 3000); }
