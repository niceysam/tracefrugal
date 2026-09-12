/* Aggregate-only session inspection. Names stay in this browser. */
(function(root){
  "use strict";
  const esc=s=>String(s??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  const t=s=>root.I18n?.t(s)||s;
  const num=n=>Number(n||0).toLocaleString(root.I18n?.locale||"en-US");
  const money=n=>"$"+Number(n||0).toFixed(4);
  const keys=["input","cached_input","cache_write","cache_write_1h","output"];
  const labels=["New input","Cache reads","Cache writes · 5 min","Cache writes · 1 hour","Output"];
  const reasons={
    native_codex_unpriced:"Native Codex pricing is not established.",
    model_not_in_price_book:"This model is not in the price book.",
    cache_lifetime_unknown:"The cache-write lifetime is unknown.",
    unsupported_speed:"This speed mode is not priced.",
    unsupported_geography:"This geography is not priced.",
    unsupported_long_context_tier:"This long-context tier is not priced.",
    price_breakdown_unavailable:"The source does not provide a price breakdown."
  };
  let aliases={};
  try{const saved=JSON.parse(root.localStorage.getItem("tracefrugal.sessionNames")||"{}");if(saved&&typeof saved==="object"&&!Array.isArray(saved))aliases=saved;}catch{}
  const shortID=s=>s.id.includes(":")?s.id.split(":").pop().slice(0,10):s.id;
  const name=(s,hidden=false)=>!hidden&&typeof aliases[s.id]==="string"?aliases[s.id]:t("Session")+" "+shortID(s);
  function saveName(id,value){
    const next={...aliases};
    if(value.trim())next[id]=value.trim().slice(0,80);else delete next[id];
    try{root.localStorage.setItem("tracefrugal.sessionNames",JSON.stringify(next));aliases=next;return true;}catch{return false;}
  }
  const input=s=>keys.slice(0,4).reduce((sum,k)=>sum+(s.tokens[k]||0),0);
  const date=s=>new Date(s).toLocaleString(root.I18n?.locale||"en-US",{year:"numeric",month:"short",day:"numeric",hour:"2-digit",minute:"2-digit"});
  function priceHTML(s){
    if(!s.prices?.length)return `<p>${t("The source does not provide a price breakdown.")}</p>`;
    return s.prices.map(g=>{
      const charged=g.rates&&g.unpriced===0;
      return `<section class="price-group"><h4 class="model-name" data-no-i18n>${esc(g.model)}</h4><p>${t("Recorded responses")}: ${num(g.requests)}</p>${charged?`<div class="table-wrap"><table><thead><tr><th>${t("Category")}</th><th>${t("Tokens")}</th><th>${t("USD / 1M tokens")}</th><th>${t("Estimated USD")}</th></tr></thead><tbody>${keys.map((k,i)=>`<tr><td>${t(labels[i])}</td><td>${num(g.tokens[k])}</td><td>${money(g.rates[k])}</td><td>${money(g.usd[k])}</td></tr>`).join("")}</tbody><tfoot><tr><th>${t("Subtotal")}</th><td colspan="2"></td><th>${money(g.known_usd)}</th></tr></tfoot></table></div>`:`<p class="notice">${t(reasons[g.reason]||reasons.price_breakdown_unavailable)} ${t("These tokens are counted; their cost is unknown.")}</p>`}</section>`;
    }).join("");
  }
  function comparisonHTML(a,b,hidden){
    if(!b)return "";
    const rows=[
      ["Recorded responses",num(a.requests),num(b.requests)],
      ["Input / response",num(Math.round(input(a)/a.requests||0)),num(Math.round(input(b)/b.requests||0))],
      ["Input P95",a.stats?num(a.stats.input_p95):"—",b.stats?num(b.stats.input_p95):"—"],
      ["Cache reads / response",num(Math.round(a.tokens.cached_input/a.requests||0)),num(Math.round(b.tokens.cached_input/b.requests||0))],
      ["Output / response",num(Math.round(a.tokens.output/a.requests||0)),num(Math.round(b.tokens.output/b.requests||0))],
      ["Estimated token cost",a.unpriced?t("Incomplete price coverage"):money(a.known_usd),b.unpriced?t("Incomplete price coverage"):money(b.known_usd)]
    ];
    return `<p class="small">${t("Different sessions can contain different tasks. This comparison does not prove savings or equal answer quality.")}</p><div class="table-wrap"><table><thead><tr><th>${t("Measure")}</th><th data-no-i18n>${esc(name(a,hidden))}</th><th data-no-i18n>${esc(name(b,hidden))}</th></tr></thead><tbody>${rows.map(r=>`<tr><th>${t(r[0])}</th><td>${r[1]}</td><td>${r[2]}</td></tr>`).join("")}</tbody></table></div>`;
  }
  function html(s,report,hidden,compareID){
    const stats=s.stats;
    const facts=[
      ["Input P50",stats?num(stats.input_p50):"—"],
      ["Input P95",stats?num(stats.input_p95):"—"],
      ["Largest input",stats?num(stats.input_max):"—"],
      ["Main responses",stats?num(stats.main_responses):"—"],
      ["Subagent responses",stats?num(stats.subagent_responses):"—"]
    ];
    const stores=(s.sources||[]).map(id=>report.sources?.find(x=>x.id===id)?.name||id);
    return `<div class="panel-heading"><div><span class="step">${t("SESSION INSPECTOR")}</span><h2 data-no-i18n>${esc(name(s,hidden))}</h2></div><button class="secondary" id="export-session">${t("Export aggregate JSON")}</button></div>
      <p class="small">${t("Observed activity")}: <span data-no-i18n>${esc(date(s.start))} → ${esc(date(s.end))}</span></p>
      <p class="small"><span>${t("Session ID")}</span>: <code>${esc(s.id)}</code> · <span>${t("Models")}</span>: <span data-no-i18n>${esc(s.models.join(", "))}</span></p>
      <p class="small"><span>${t("Log stores")}</span>: <span data-no-i18n>${esc(stores.join(", ")||"—")}</span>${!hidden?` · <span>${t("Project")}</span>: <span data-no-i18n>${esc(s.project)}</span>`:""}</p>
      ${!hidden?`<div class="alias-form"><label for="session-alias">${t("Local session name")}</label><input id="session-alias" maxlength="80" value="${esc(aliases[s.id]||"")}" placeholder="${t("For example: payment API investigation")}"><button class="secondary" id="save-session-name">${t("Save name")}</button></div><p class="small">${t("Stored only in this browser. Leave blank to reset. No conversation text is imported.")}</p>`:""}
      <div class="inspector-stats">${facts.map(([k,v])=>`<div><span>${t(k)}</span><strong>${v}</strong></div>`).join("")}</div>
      <p class="small">${t("P50 is the median input; P95 is the input at the 95th percentile. Cache reads and writes are included. Statistics cover all responses in the selected period.")}</p>
      <p class="notice">${t("Responses include tool loops and recorded subagents, not just user questions. Processed tokens count repeated context; they are not the context-window size.")}</p>
      <h3>${t("How this cost was calculated")}</h3><p>${t("For each response: tokens ÷ 1,000,000 × the category rate. Then add all categories and responses.")}</p>
      <p class="small">${t("Standard list-price estimate, not an invoice. Subscription plans, provider discounts, credits and taxes are not applied.")}</p>
      <p class="small">${t("Price source")}: <span>${esc(t(report.price_label))}</span></p>${priceHTML(s)}
      <h3>${t("Compare another session")}</h3><label for="compare-session">${t("Choose a comparison")}</label><select id="compare-session" data-no-i18n><option value="">${t("Choose a comparison")}</option>${report.sessions.filter(x=>x.id!==s.id).map(x=>`<option value="${esc(x.id)}" ${x.id===compareID?"selected":""}>${esc(name(x,hidden))}</option>`).join("")}</select>
      ${comparisonHTML(s,report.sessions.find(x=>x.id===compareID&&x.id!==s.id),hidden)}`;
  }
  function exportData(s,report){
    // Explicit allowlist: no project paths, local aliases, prompts, or raw events.
    return {schema:"tracefrugal.session.v1",period:{from:report.from,until:report.until},session_id:s.id,observed_start:s.start,observed_end:s.end,models:s.models,requests:s.requests,tokens:s.tokens,known_usd:s.known_usd,unpriced:s.unpriced,stats:s.stats,prices:s.prices,price_label:report.price_label};
  }
  root.TraceInspector={shortID,name,saveName,html,exportData};
})(globalThis);
