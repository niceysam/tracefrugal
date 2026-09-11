/* Local Claude Code dashboard. No remote requests, dependencies, or telemetry. */
"use strict";
const boot = JSON.parse(document.getElementById("bootstrap").textContent);
const $ = id => document.getElementById(id);
const esc = value => String(value ?? "").replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
const number = value => Number(value || 0).toLocaleString("en-US");
const compact = value => new Intl.NumberFormat("en-US", {notation:"compact", maximumFractionDigits:1}).format(value || 0);
const usd = value => "$" + Number(value || 0).toFixed(2);
const tokens = t => t.input + t.cached_input + t.cache_write + t.cache_write_1h + t.output;
const date = value => new Date(value).toLocaleString("en-US",{month:"short",day:"numeric",hour:"2-digit",minute:"2-digit"});
const demo = boot.mode === "claude-demo";
let state, days = 7, unit = "tokens", view = "overview", selected = "", busy = false, restoreID = "", recipeID = "", sequence = 0, sort = "recent", hideNames = false;
const inputTokens = s => tokens(s.tokens)-s.tokens.output;
const per = (n,d) => d ? number(Math.round(n/d)) : "—";
const bytes = n => n >= 1e6 ? (n/1e6).toFixed(1)+" MB" : n >= 1000 ? (n/1000).toFixed(1)+" kB" : number(Math.round(n))+" B";
const recipe = id => state.recipes.find(r=>r.id===id);

function notice(text, error = false) { $("notice").textContent = text; $("notice").classList.toggle("error", error); }
function showView(name) {
  if (!["overview","changes","setup"].includes(name)) return;
  view = name;
  document.querySelectorAll(".view").forEach(e => e.hidden = e.id !== name);
  document.querySelectorAll("[data-view]").forEach(e => {
    if (e.dataset.view === name) e.setAttribute("aria-current","page"); else e.removeAttribute("aria-current");
  });
}
function metric(label, value, note) {
  return `<article class="metric"><span class="metric-label">${esc(label)}</span><strong>${esc(value)}</strong><small>${esc(note)}</small></article>`;
}
function estimate(s) { return s.unpriced ? usd(s.known_usd) + " known" : usd(s.known_usd); }
function project(s) { return hideNames ? "Project " + s.id.slice(0,6) : s.project; }
function activeChange() { return state?.changes.slice().reverse().find(c => ["saved","prepared"].includes(c.status)); }
function render() {
  if (!state) return;
  const s = state.summary, total = tokens(s.tokens), input = total-s.tokens.output;
  $("source").textContent = demo ? "Sample workspace" : "Claude Code · local";
  $("source").classList.toggle("demo", demo);
  $("demo-banner").hidden = !demo;
  $("freshness").textContent = demo ? "Explore sample sessions. Install to see your own usage automatically." : `Last read ${date(state.until)} · updates every 30 seconds · this computer only`;
  document.querySelectorAll("[data-days]").forEach(b => b.setAttribute("aria-pressed", String(Number(b.dataset.days) === days)));
  document.querySelectorAll("[data-unit]").forEach(b => b.setAttribute("aria-pressed", String(b.dataset.unit === unit)));
  $("empty").hidden = s.requests > 0;
  $("metrics").innerHTML = metric("Estimated token cost", estimate(s), s.unpriced ? `${number(s.unpriced)} of ${number(s.requests)} requests unpriced` : "Standard list rates · not a subscription bill") +
    metric("Input / response", compact(input/s.requests || 0), `${compact(input)} input tokens processed in total`) +
    metric("Input from cache", input ? (s.tokens.cached_input/input*100).toFixed(1)+"%" : "—", "Cached input costs less; it still counts") +
    metric("Output / response", compact(s.tokens.output/s.requests || 0), `${number(s.requests)} responses · ${number(state.sessions.length)} sessions`);
  renderChart();
  renderSessions();
  $("findings").innerHTML = state.findings.map(f => `<div class="finding"><h3>${esc(f.title)}</h3><p>${esc(f.detail)}</p><p>${esc(f.action)}</p>${f.session ? `<button class="text-button" data-session="${esc(f.session)}">Inspect this session →</button>` : ""}</div>`).join("") || '<p>Usage signals will appear after requests are recorded.</p>';
  const c = activeChange();
  $("trial-card").innerHTML = c
    ? `<span class="step">CONTEXT TRIAL</span><h2>${c.hours.length===24 ? "Your day is ready to review." : "One change. Watch what happens."}</h2><p>${esc(recipe(c.recipe)?.title || c.recipe)}</p><p>${c.hours.length} of 24 complete hours recorded. ${c.rule_state==="present" ? "The rule file is present; verify it is loaded in your next Claude session." : "The trial rule is "+esc(c.rule_state || "unverified")+". Check it before interpreting results."}</p><div class="intro-cta"><button class="primary" data-view="changes">Compare tokens &amp; satisfaction →</button><button class="text-button" data-restore="${esc(c.id)}">Undo change</button></div>`
    : `<span class="step">A PRACTICAL EXPERIMENT</span><h2>Try one rule for a day.</h2><p>Start with the biggest observed source below. Rate the answers you get today, add a small context instruction, and compare tomorrow. Undo it if the answers become less useful.</p><p class="small">No model calls are made by TraceFrugal. The baseline is the previous 24 hours, across projects in this Claude config directory.</p>`;
  renderDiagnosis();
  renderChanges();
  const h = state.health;
  $("coverage").textContent = `${number(h.files)} local transcript files · ${number(h.duplicates)} repeated response blocks deduplicated · ${number(h.invalid)} invalid records · ${number(h.partial)} incomplete final lines · ${number(h.unreadable)} unreadable files · ${number(h.conflicts)} conflicting usage snapshots. ` +
    "Incomplete or missing logs can undercount usage. Prices: "+state.price_label+". Special or unknown pricing remains unpriced. Cached tokens are included in processed tokens; this is not context-window occupancy. Prompts and tool outputs are not displayed or uploaded.";
  showView(view);
}
function renderDiagnosis() {
  const d=state.diagnostics, s=state.summary, input=inputTokens(s), share=input ? s.tokens.cached_input/input : 0;
  $("diagnosis").innerHTML=`<div class="diagnosis-grid"><div><h3>${per(input,s.requests)} input → ${per(s.tokens.output,s.requests)} output</h3><p>Average tokens per recorded response. Input includes instructions, prior conversation, tool definitions and results. The same context may be processed on many requests, even when the answer is short.</p></div><div><h3>${(share*100).toFixed(1)}% of input was cached</h3><p>${share>=.7 ? "Most input was reused from cache. That is a lower-priced read, not evidence of waste. A large conversation can still be replayed many times." : "Much of the input was newly processed or written to cache. New sessions, changing prefixes, compaction, or expiry can explain this; usage logs alone do not prove which."}</p></div><div><h3>${per(s.requests,d.human_turns)} responses per user turn</h3><p>${number(d.human_turns)} observed main-session user turns · ${number(d.tool_calls)} tool calls · ${number(d.repeated_calls)} repeated identical calls. Tool loops and subagents add requests beyond your visible questions. Repeats may be legitimate polling.</p></div></div>`;
  const max=Math.max(1,...d.tools.map(t=>t.bytes));
  $("tool-sources").innerHTML=`<h3>Which tools returned the most data?</h3><p class="small">${bytes(d.result_bytes)} observed serialized tool-result data · ${bytes(d.mcp_bytes)} from MCP · ${number(d.large_results)} results ≥ 20 kB. Bytes help find large sources; they are not token attribution or the full request context.</p>${d.tools.length ? `<div class="table-wrap"><table><thead><tr><th>Tool</th><th>Result size</th><th>Calls</th><th>Repeated</th></tr></thead><tbody>${d.tools.slice(0,8).map(t=>`<tr><td class="tool-name">${esc(t.name)}</td><td><span class="source-meter"><i style="width:${t.bytes/max*100}%"></i></span>${bytes(t.bytes)}</td><td>${number(t.calls)}</td><td>${number(t.repeated)}</td></tr>`).join("")}</tbody></table></div>` : '<p>No attributable tool results in these logs. A missing result is not a zero-size input.</p>'}<details><summary>What about MCP schemas and always-loaded instructions?</summary><p>These logs do not split input tokens by system prompt, memory, MCP schemas, or conversation. Run <code>/context</code> in Claude Code for its breakdown. Modern Claude Code defers MCP tool definitions when tool search is available; connecting a server does not prove all its schemas are resent.</p><p>Use <code>/mcp</code> to review unused servers. Check whether tool discovery is available in your provider configuration. Keep always-loaded memory concise; put specialist instructions in scoped rules or skills. TraceFrugal does not disable your servers or rewrite your memory.</p></details>`;
  const reasons={
    "tool-results": `${number(d.large_results)} large results observed. Fetch selected lines, fields, and bounded results before expanding.`,
    "mcp": `${bytes(d.mcp_bytes)} observed MCP result data. Use discovery where supported and narrow query scope.`,
    "turns": `${number(d.repeated_calls)} repeated identical calls. Batch independent reads and stop redundant lookups.`
  };
  const suggested=d.mcp_bytes>d.result_bytes/2?"mcp":d.large_results?"tool-results":d.repeated_calls?"turns":"";
  $("recipes").innerHTML=state.recipes.slice().sort((a,b)=>(b.id===suggested)-(a.id===suggested)).map(r=>`<div class="recipe">${r.id===suggested?'<span class="step">SUGGESTED FIRST · OBSERVED SIGNAL</span>':""}<h3>${esc(r.title)}</h3><p>${esc(reasons[r.id] || r.reason)}</p><button class="${r.id===suggested?"primary":"secondary"}" data-try="${esc(r.id)}" ${activeChange() || !s.requests ? "disabled" : ""}>Preview 24-hour trial →</button></div>`).join("");
}
function renderChart() {
  if (!state) return;
  const session = state.sessions.find(s => s.id === selected);
  const rows = session
    ? session.timeline.slice(-60).map((r,i,a) => ({...r, start:r.time, requests:1, known_usd:r.cost ?? 0, unpriced:r.cost == null ? 1:0, label:String(session.requests-a.length+i+1)}))
    : state.buckets.map(b => ({...b, label:days === 1 ? new Date(b.start).getUTCHours()+":00" : new Date(b.start).toLocaleDateString("en-US",{timeZone:"UTC",month:"short",day:"numeric"})}));
  $("chart-title").textContent = session ? project(session) + " · request by request" : "Usage over time";
  $("chart-caption").textContent = session ? `Latest ${rows.length} displayed responses from this session. Subagents included. Repeated input is expected as a conversation grows.` : (days === 1 ? "Hourly" : "Daily")+" buckets in UTC. First and last buckets may be partial.";
  const width = Math.max(300, $("chart").clientWidth || 900), left = 57, right = 15, bottom = 216;
  const known = r => unit === "tokens" || r.unpriced === 0;
  const value = r => unit === "tokens" ? tokens(r.tokens) : r.known_usd;
  const max = Math.max(...rows.filter(known).map(value), unit === "tokens" ? 1 : .01)*1.1;
  const y = v => bottom - v/max*172, step = (width-left-right)/Math.max(1,rows.length), bar = Math.min(step*.7, 70);
  const parts = r => [r.tokens.input+r.tokens.cache_write+r.tokens.cache_write_1h, r.tokens.cached_input, r.tokens.output];
  const colors = ["#4269de","#79bea2","#d7a44b"];
  let svg = `<svg viewBox="0 0 ${width} 257" role="img" aria-label="${unit === "tokens" ? "Stacked processed tokens" : "Estimated USD"} over ${session ? "requests" : "time"}. Values below.">`;
  for(let i=0;i<4;i++) {
    const v=max*i/3;
    svg+=`<line class="grid" x1="${left}" x2="${width-right}" y1="${y(v)}" y2="${y(v)}"/><text x="${left-8}" y="${y(v)+4}" text-anchor="end">${unit === "tokens" ? compact(v) : usd(v)}</text>`;
  }
  rows.forEach((r,i) => {
    const x=left+step*(i+.5), caption = session ? `Response ${r.label} · ${date(r.start)}` : new Date(r.start).toISOString();
    if (!known(r)) {
      svg+=`<circle class="request-zero" cx="${x}" cy="${bottom-5}" r="4"><title>${esc(caption)}: incomplete price coverage, not zero cost</title></circle>`;
    } else {
      const segments = unit === "tokens" ? parts(r) : [value(r)];
      let sum=0;
      segments.forEach((n,j) => {
        sum+=n;
        svg+=`<rect class="usage-bar" x="${x-bar/2}" y="${y(sum)}" width="${bar}" height="${n/max*172}" rx="2" fill="${unit === "tokens" ? colors[j] : "#157755"}"><title>${esc(caption)}: ${unit === "tokens" ? number(value(r))+" tokens" : usd(value(r))}</title></rect>`;
      });
    }
    const every = Math.max(1,Math.ceil(rows.length/(width < 500 ? 4:8)));
    if(i%every===0 || i===rows.length-1) svg+=`<text x="${x}" y="243" text-anchor="middle">${esc(r.label)}</text>`;
  });
  $("chart").innerHTML = svg+"</svg>"+(session ? '<button class="text-button" data-session="">← Back to all sessions</button>' : "");
  $("legend").innerHTML = unit === "tokens" ? '<span><i class="dot blue"></i>New input + cache writes</span><span><i class="dot" style="background:#79bea2"></i>Cached input</span><span><i class="dot" style="background:#d7a44b"></i>Output</span>' : '<span><i class="dot green"></i>Complete price coverage</span><span>○ Unpriced data · not zero cost</span>';
  $("chart-values").innerHTML = `<table class="readable-table"><thead><tr><th>Time</th><th>New input</th><th>Cache reads</th><th>Cache writes</th><th>Output</th><th>Est. USD</th></tr></thead><tbody>${rows.map(r=>`<tr><td>${esc(date(r.start))}</td><td>${number(r.tokens.input)}</td><td>${number(r.tokens.cached_input)}</td><td>${number(r.tokens.cache_write+r.tokens.cache_write_1h)}</td><td>${number(r.tokens.output)}</td><td>${esc(estimate(r))}</td></tr>`).join("")}</tbody></table>`;
}
function renderSessions() {
  const sessions = state.sessions.slice().sort((a,b) => sort === "tokens" ? tokens(b.tokens)-tokens(a.tokens) : sort === "cost" ? b.known_usd-a.known_usd : new Date(b.end)-new Date(a.end));
  $("sessions").innerHTML = sessions.length ? `<table><thead><tr><th>Project / session</th><th>Responses</th><th>Tokens</th><th>Est. cost</th></tr></thead><tbody>${sessions.map(s=>`<tr class="${s.id===selected ? "session-selected" : ""}"><td><button class="session" data-session="${esc(s.id)}">${esc(project(s))}<span class="sub">${esc(date(s.end))} · ${esc(s.id.slice(0,6))}</span></button></td><td>${number(s.requests)}</td><td>${compact(tokens(s.tokens))}</td><td>${esc(estimate(s))}${s.unpriced ? '<span class="sub">Partial price coverage</span>' : ""}</td></tr>`).join("")}</tbody></table>` : '<div class="empty">Sessions appear after Claude Code records usage.</div>';
}
function renderChanges() {
  $("change-count").textContent = state.changes.length;
  $("changes-list").innerHTML = state.changes.length ? state.changes.slice().reverse().map(c => {
    const before=c.baseline, after=c.after, complete=c.hours.length===24;
    const avgBefore=inputTokens(before)/before.requests, avgAfter=after.requests ? inputTokens(after)/after.requests : null;
    const delta=avgAfter===null || !avgBefore ? "Waiting for usage" : (Math.abs(avgAfter/avgBefore-1)*100).toFixed(1)+"% "+(avgAfter<avgBefore?"lower":"higher");
    const duration=(new Date(after.until)-new Date(c.started))/3600000;
    const rows=[{label:"Before · 24h",...before},...c.hours.map((h,i)=>({label:`Hour ${i+1} · ${date(h.start)}`,...h}))];
    const maximum=Math.max(1,...rows.filter(r=>r.requests).map(r=>inputTokens(r)/r.requests));
    const bars=rows.map((r,i)=>`<div class="hour-bar"><span>${i===0?"Before":"Hour "+i}</span><div><i style="width:${r.requests ? inputTokens(r)/r.requests/maximum*100 : 0}%"></i></div><strong>${r.requests ? compact(inputTokens(r)/r.requests) : "No usage"}</strong></div>`).join("");
    const compare=[
      ["Observed user turns",number(before.diagnostics.human_turns),number(after.diagnostics.human_turns)],
      ["Recorded responses",number(before.requests),number(after.requests)],
      ["Total processed tokens",compact(tokens(before.tokens)),compact(tokens(after.tokens))],
      ["Input / response",per(inputTokens(before),before.requests),per(inputTokens(after),after.requests)],
      ["Cache reads / response",per(before.tokens.cached_input,before.requests),per(after.tokens.cached_input,after.requests)],
      ["Input / user turn",per(inputTokens(before),before.diagnostics.human_turns),per(inputTokens(after),after.diagnostics.human_turns)],
      ["Responses / user turn",before.diagnostics.human_turns ? (before.requests/before.diagnostics.human_turns).toFixed(1):"—",after.diagnostics.human_turns ? (after.requests/after.diagnostics.human_turns).toFixed(1):"—"],
      ["Tool-result bytes / response",before.requests ? bytes(before.diagnostics.result_bytes/before.requests):"—",after.requests ? bytes(after.diagnostics.result_bytes/after.requests):"—"],
      ["Output / response",per(before.tokens.output,before.requests),per(after.tokens.output,after.requests)],
      ["Est. cost / response",before.requests && !before.unpriced ? "$"+(before.known_usd/before.requests).toFixed(4):"Unpriced / no usage",after.requests && !after.unpriced ? "$"+(after.known_usd/after.requests).toFixed(4):"Unpriced / no usage"],
      ["Answer & reasoning satisfaction",c.before_rating+" / 5",c.after_rating ? c.after_rating+" / 5":"Not rated"]
    ];
    const quality=c.after_rating ? c.after_rating<c.before_rating ? `Your satisfaction fell: ${c.before_rating}/5 → ${c.after_rating}/5. Consider undoing the rule, even if input is lower.` : `Your satisfaction: ${c.before_rating}/5 → ${c.after_rating}/5. Check similar tasks before deciding this rule helped.` : "Rate the usefulness of the answers and reasoning before deciding to keep the rule.";
    return `<article class="history-detail"><div class="change-top"><div><h2>${esc(recipe(c.recipe)?.title || c.recipe)}</h2><p class="small">Started ${esc(date(c.started))} · ${number(c.rule_bytes)} instruction bytes added · all projects in this Claude directory</p></div><span class="pill">${c.status==="restored" ? "Rule removed" : complete ? "24 hours ready" : "Observing"}</span></div><div class="trial-verdict ${c.after_rating && c.after_rating<c.before_rating ? "quality-down" : ""}"><strong>${esc(delta)} input per response</strong><p>${esc(quality)}</p></div>${comparisonGraph(before,after)}<p class="small">Before: 24 hours. After: ${Number.isFinite(duration)?Math.max(0,duration).toFixed(1):"0"} hours${complete?"":" · partial"}. This is an observational comparison, not a controlled savings result. Saving the rule does not prove it was loaded or followed.</p><div class="table-wrap"><table><thead><tr><th>Measure</th><th>Before</th><th>After</th></tr></thead><tbody>${compare.map(r=>`<tr>${r.map((v,i)=>`<${i?"td":"th"}>${esc(v)}</${i?"td":"th"}>`).join("")}</tr>`).join("")}</tbody></table></div><div class="rating-control"><label for="rating-${esc(c.id)}">After trying it, how satisfied are you?</label><select id="rating-${esc(c.id)}"><option value="">Choose 1–5</option>${[1,2,3,4,5].map(n=>`<option value="${n}" ${c.after_rating===n?"selected":""}>${n} · ${["Very dissatisfied","Dissatisfied","Mixed","Satisfied","Very satisfied"][n-1]}</option>`).join("")}</select><button class="secondary" data-rate="${esc(c.id)}" ${after.requests?"":"disabled"}>Save satisfaction</button></div><details class="hour-bars"><summary>Hourly graph · input tokens per response</summary><p class="small">Baseline average vs. each completed hour. Zero recorded requests means no observation, not savings.</p>${bars}</details><p class="small">The rule remains until removed. Hourly snapshots are collected for 24 hours; your ratings and history are retained.</p>${["saved","prepared"].includes(c.status)?`<button class="secondary" data-restore="${esc(c.id)}">Undo · remove trial rule</button>`:`<p class="small">Removed ${esc(date(c.finished))}. Start a fresh Claude session to stop using the removed instruction.</p>`}</article>`;
  }).join("") : '<article class="panel onboarding"><h2>No context trial yet.</h2><p>Choose a recommendation from Overview, rate your current answer satisfaction, and preview the exact rule before applying it.</p><button class="primary" data-view="overview">Choose a context improvement →</button></article>';
}
function comparisonGraph(before,after) {
  const rows=[before,after], max=Math.max(1,...rows.filter(r=>r.requests).map(r=>inputTokens(r)/r.requests));
  return '<div class="hour-bars"><h3>Input tokens per response</h3>'+rows.map((r,i)=>`<div class="hour-bar"><span>${i?"After":"Before"}</span><div><i style="width:${r.requests?inputTokens(r)/r.requests/max*100:0}%"></i></div><strong>${r.requests?compact(inputTokens(r)/r.requests):"No usage"}</strong></div>`).join("")+"</div>";
}
async function refresh() {
  const current = ++sequence;
  try {
    const fresh = demo ? sample(days) : await (async()=>{
      const response=await fetch("api/state?days="+days,{cache:"no-store"});
      if(!response.ok) throw new Error(await response.text());
      return response.json();
    })();
    if(current!==sequence) return;
    if(demo && state) fresh.changes=state.changes;
    state=fresh;
    render();
  } catch(error) { notice("Could not refresh. "+error.message+" Any visible figures are from the previous read.",true); }
}
async function change(action, id = "", rating = 0) {
  if(busy) return;
  busy=true;
  $("apply-trial").disabled=$("restore-trial").disabled=true;
  try {
    if(demo) {
      if(action==="try") {
        const now=Date.now();
        const before={...state.summary,diagnostics:state.diagnostics,start:new Date(now-172800000).toISOString(),until:new Date(now-86400000).toISOString()};
        const hours=Array.from({length:24},(_,i)=>{
          const t={...before.tokens}; for(const key in t) t[key]=Math.round(t[key]/24*(key==="output"?1:.75));
          return {requests:Math.max(1,Math.round(before.requests/24)),tokens:t,known_usd:before.known_usd/24*.8,unpriced:0,diagnostics:{...before.diagnostics,human_turns:2,result_bytes:2000},start:new Date(now-86400000+i*3600000).toISOString(),until:new Date(now-86400000+(i+1)*3600000).toISOString()};
        });
        const after={...hours[0],requests:0,known_usd:0,tokens:{input:0,cached_input:0,cache_write:0,cache_write_1h:0,output:0},diagnostics:{...before.diagnostics,human_turns:0,result_bytes:0},start:hours[0].start,until:hours[23].until};
        for(const h of hours) {after.requests+=h.requests;after.known_usd+=h.known_usd;after.diagnostics.human_turns+=h.diagnostics.human_turns;after.diagnostics.result_bytes+=h.diagnostics.result_bytes;for(const k in after.tokens)after.tokens[k]+=h.tokens[k];}
        state.changes.push({id:"sample-"+now,started:hours[0].start,recipe:recipeID,status:"saved",baseline:before,after,hours,before_rating:rating,after_rating:0,rule_bytes:recipe(recipeID).rule.length,rule_state:"present"});
      } else if(action==="rate") {
        state.changes.find(c=>c.id===id).after_rating=rating;
      } else {
        const c=state.changes.find(c=>c.id===restoreID);
        if(c) { c.status="restored"; c.finished=new Date().toISOString(); }
      }
    } else {
      const body=new URLSearchParams({token:boot.token,recipe:recipeID,rating:String(rating),id:action==="restore"?restoreID:id});
      const response=await fetch("api/"+action,{method:"POST",body});
      if(!response.ok) throw new Error(await response.text());
      await refresh();
    }
    notice(demo ? "Sample only: "+(action==="try"?"24 hours simulated so you can explore the comparison. No real savings or rule changes.":action==="rate"?"Satisfaction saved for this demo.":"Trial rule removal simulated. No real files changed.") : action==="try" ? "Context rule saved. Start a new Claude Code session and verify it in /context. Leave this app running for hourly observations." : action==="rate" ? "Your satisfaction rating is saved with this trial." : "Trial rule removed. Start a fresh Claude Code session to stop using that instruction.");
    showView("changes");
    render();
  } catch(error) { notice(error.message,true); }
  finally { busy=false; $("apply-trial").disabled=$("restore-trial").disabled=false; }
}
function sample(period) {
  const now=Date.now(), n=period===1?24:period, step=period===1?3600000:86400000;
  const zero=()=>({requests:0,tokens:{input:0,cached_input:0,cache_write:0,cache_write_1h:0,output:0},known_usd:0,unpriced:0});
  const summary=zero(), buckets=[], sessions=[];
  const pattern=[.28,.48,.32,.81,.59,.95,.47];
  for(let i=0;i<n;i++) {
    const scale=pattern[i%7], start=new Date(Math.floor(now/step)*step-(n-1-i)*step).toISOString();
    const s={requests:Math.round(40*scale),tokens:{input:Math.round(22000*scale),cached_input:Math.round(710000*scale),cache_write:Math.round(65000*scale),cache_write_1h:0,output:Math.round(23000*scale)},known_usd:0,unpriced:0};
    s.known_usd=(s.tokens.input*3+s.tokens.cached_input*.3+s.tokens.cache_write*3.75+s.tokens.output*15)/1e6;
    buckets.push({...s,start});
    for(const k in s.tokens) summary.tokens[k]+=s.tokens[k];
    summary.requests+=s.requests; summary.known_usd+=s.known_usd;
    const requests=[];
    for(let j=0;j<s.requests;j++) {
      const t={}; for(const k in s.tokens) t[k]=Math.floor(s.tokens[k]/s.requests)+(j<s.tokens[k]%s.requests?1:0);
      requests.push({time:new Date(new Date(start).getTime()+j*60000).toISOString(),tokens:t,cost:(t.input*3+t.cached_input*.3+t.cache_write*3.75+t.output*15)/1e6,effort:"high",model:"claude-sonnet-4-6",subagent:false});
    }
    sessions.push({...s,id:"demo-session-"+i,project:["website","api-service","weekend-project"][i%3],start,end:start,models:["claude-sonnet-4-6"],timeline:requests});
  }
  return {mode:"claude",from:new Date(now-period*86400000).toISOString(),until:new Date(now).toISOString(),summary,buckets,sessions:sessions.reverse(),changes:[],recipes:[
    {id:"tool-results",title:"Keep large tool results out of the conversation",rule:"Use narrow file ranges, query filters, requested fields, and result limits before fetching data. Keep full artifacts locally and return relevant excerpts, counts, errors, and paths. Expand when needed. Preserve evidence and verification."},
    {id:"mcp",title:"Use MCP discovery and smaller result sets",rule:"When MCP tool discovery is available, discover only tools needed for this step. Prefer specific queries, fields, pagination, and limits. Reuse relevant results. Expand when needed. Do not disable servers or skip required checks."},
    {id:"turns",title:"Avoid redundant tool round trips",rule:"Batch independent reads. Reuse results unless state changed. Poll at meaningful intervals. Finish once completion criteria and required checks pass. Preserve approvals. Offer a handoff for unrelated tasks; do not clear automatically."}
  ],diagnostics:{human_turns:Math.max(1,Math.round(summary.requests/5)),tool_calls:87,result_bytes:840000,mcp_bytes:720000,large_results:12,repeated_calls:9,tools:[{name:"mcp__docs__search",calls:24,bytes:720000,repeated:6},{name:"Read",calls:42,bytes:90000,repeated:1},{name:"Bash",calls:21,bytes:30000,repeated:2}]},price_label:"SYNTHETIC usage at example standard list rates",health:{files:n,duplicates:34,invalid:0,partial:0,unreadable:0,conflicts:0},findings:[{title:"Start with a session you recognize",detail:"Select website or api-service below to see individual requests. Toggle Tokens and Est. cost.",action:"In the local app these are your recorded sessions. The sample numbers here are illustrative, not measured savings."}]};
}
document.addEventListener("click", event => {
  const nav=event.target.closest("[data-view]"); if(nav) showView(nav.dataset.view);
  const range=event.target.closest("[data-days]"); if(range) { days=Number(range.dataset.days); selected=""; refresh(); }
  const choice=event.target.closest("[data-unit]"); if(choice) { unit=choice.dataset.unit; render(); }
  const session=event.target.closest("[data-session]"); if(session) { selected=session.dataset.session; render(); $("chart-title").scrollIntoView?.({block:"center",behavior:"smooth"}); }
  const trial=event.target.closest("[data-try]");
  if(trial && !activeChange() && !busy) { recipeID=trial.dataset.try; const r=recipe(recipeID); if(r){$("trial-title").textContent=r.title; $("trial-rule").textContent=r.rule; $("before-rating").value=""; $("trial-dialog").showModal();} }
  const rate=event.target.closest("[data-rate]");
  if(rate && !busy) {const rating=Number($("rating-"+rate.dataset.rate).value);if(rating>=1&&rating<=5) change("rate",rate.dataset.rate,rating);else notice("Choose a satisfaction rating from 1 to 5.",true);}
  const restore=event.target.closest("[data-restore]"); if(restore && !busy) { restoreID=restore.dataset.restore; $("restore-dialog").showModal(); }
  const close=event.target.closest("[data-close]"); if(close) $(close.dataset.close).close();
});
$("apply-trial").addEventListener("click",()=>{const rating=Number($("before-rating").value);if(rating<1||rating>5){$("before-rating").focus();return;}$("trial-dialog").close();change("try","",rating);});
$("restore-trial").addEventListener("click",()=>{$("restore-dialog").close();change("restore");});
$("session-sort").addEventListener("change",event=>{sort=event.target.value;renderSessions();});
$("hide-names").addEventListener("change",event=>{hideNames=event.target.checked;render();});
const windows=typeof navigator !== "undefined" && /Win/.test(navigator.platform);
$("launch-command").textContent=windows ? ".\\tracefrugal.exe" : "./tracefrugal";
$("copy-command").addEventListener("click",async()=>{
  try { await navigator.clipboard.writeText($("launch-command").textContent); $("copy-command").textContent="Copied"; }
  catch { notice("Select and copy the command shown above."); }
});
if(typeof window !== "undefined") window.addEventListener("resize",renderChart);
refresh();
if(!demo) setInterval(()=>{if(!busy && !$("trial-dialog").open && !$("restore-dialog").open) refresh();},30000);
