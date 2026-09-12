/* Usage economics, not task-quality inference. Shared by local and sample views. */
(function(root){
  "use strict";
  const keys=["input","cached_input","cache_write","cache_write_1h","output"];
  const safe=n=>Number.isFinite(n)&&n>=0?n:0;
  const esc=s=>String(s??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
  const input=s=>keys.slice(0,4).reduce((n,k)=>n+safe(s?.tokens?.[k]),0);
  const priced=s=>safe(s?.requests)>0&&s.unpriced===0&&Number.isFinite(s.known_usd)&&s.known_usd>=0;
  const delta=(a,b)=>a>0?(b/a-1)*100:null;
  const duration=s=>{const a=Date.parse(s?.from||s?.start),b=Date.parse(s?.until||s?.end);return Number.isFinite(a)&&b>=a?(b-a)/3600000:null;};
  const agents=s=>Number.isInteger(s?.main_responses)&&Number.isInteger(s?.subagent_responses)&&s.main_responses>=0&&s.subagent_responses>=0&&s.requests>0&&s.main_responses+s.subagent_responses===s.requests?`${s.main_responses} / ${s.subagent_responses}`:"—";
  function incomplete(report){
    const bad=h=>Object.entries(h||{}).some(([k,v])=>["missing","invalid","partial","unreadable","conflicts"].includes(k)&&v>0);
    return !!report?.log_error||bad(report?.health)||(report?.sources||[]).some(s=>
      (!report.selected_source||report.selected_source===s.id)&&(bad(s.health)||s.unresolved_usage_events>0));
  }
  const language=lang=>lang||root.I18n?.language||"en";
  function format(lang){
    const ko=language(lang)==="ko";
    return {t:(en,kr)=>ko?kr:en,n:n=>new Intl.NumberFormat(ko?"ko-KR":"en-US",{maximumFractionDigits:1}).format(n),
      money:n=>"$"+n.toFixed(4),pct:n=>n===null?"—":(n>0?"+":"")+new Intl.NumberFormat(ko?"ko-KR":"en-US",{maximumFractionDigits:1}).format(n)+"%"};
  }
  function measure(s){
    const total=input(s),requests=safe(s?.requests);
    return {input:total,requests,average:requests?total/requests:null,cache:total?safe(s.tokens.cached_input)/total:null,
      cost:priced(s)?s.known_usd:null,hours:duration(s)};
  }
  // Signals describe observed changes. They never declare a causal saving or
  // infer task completion, retry counts, active duration, or quality from usage.
  function compare(a,b,options={}){
    const x=measure(a),y=measure(b),signals=[];
    if(!x.requests||!y.requests)return {before:x,after:y,signals:["no_usage"]};
    if(x.average>y.average&&y.input>x.input)signals.push("more_total_input");
    if(x.cost!==null&&y.cost!==null&&y.input<x.input&&y.cost>x.cost)signals.push("less_input_more_cost");
    if(y.requests>x.requests)signals.push("more_responses");
    if(x.cost===null||y.cost===null)signals.push("price_gap");
    if(x.hours!==null&&y.hours!==null&&Math.abs(x.hours-y.hours)>.01)signals.push("different_windows");
    const models=s=>Array.isArray(s.models)?[...new Set(s.models)].sort().join("\n"):null;
    if(models(a)!==null&&models(b)!==null&&models(a)!==models(b))signals.push("different_models");
    if(options.outcome==="regression"||(options.beforeRating>0&&options.afterRating>0&&options.afterRating<options.beforeRating))signals.push("quality_regression");
    else if(!(options.beforeRating>0&&options.afterRating>0)||!options.outcome||options.outcome==="unknown")signals.push("quality_unknown");
    if(options.incomplete)signals.push("coverage_gap");
    return {before:x,after:y,signals};
  }
  const messages={
    no_usage:["Waiting for both observations","전후 사용 기록을 기다립니다","No recorded usage is not a saving.","사용 기록이 없다고 절감한 것은 아닙니다."],
    more_total_input:["Smaller requests, more input overall","한 번의 입력은 줄었지만 전체 입력은 늘었습니다","The average fell, but more responses outweighed it. Inspect tool reads and repeated calls before keeping the change.","평균은 줄었지만 응답 횟수 증가가 이를 상쇄했습니다. 도구 결과 재조회와 반복 호출을 확인하세요."],
    less_input_more_cost:["Fewer input tokens, higher estimated cost","입력 토큰은 줄었지만 추정 비용은 늘었습니다","Check cache reads versus new input and cache writes, output usage, and model rates. These totals do not prove a cache miss.","캐시 읽기와 새 입력·캐시 쓰기, 출력량, 모델 단가를 함께 확인하세요. 이 합계만으로 캐시 실패를 확정하지 않습니다."],
    more_responses:["More model responses were recorded","모델 응답 횟수가 늘었습니다","All recorded responses count, including tool loops and subagents. Extra work may be useful; counts alone do not identify retries.","도구 루프와 기록된 서브에이전트도 합산합니다. 필요한 추가 작업일 수 있으며, 횟수만으로 재시도를 구분하지 않습니다."],
    price_gap:["Total cost cannot be compared","전체 비용을 비교할 수 없습니다","At least one side has incomplete pricing. Compare recorded tokens; unknown prices are not zero.","한쪽 이상의 가격 정보가 불완전합니다. 관측 토큰을 비교하되 미산정 금액을 0으로 보지 마세요."],
    different_windows:["Observation lengths differ","관측 기간이 다릅니다","A shorter window can contain less work. Wait for comparable work; do not extrapolate a partial day into savings.","짧은 기간에는 작업도 적을 수 있습니다. 비교할 작업이 모일 때까지 기다리세요. 일부 시간으로 하루 절감을 예측하지 않습니다."],
    different_models:["The model mix changed","사용 모델 구성이 달라졌습니다","Rates and task behavior may differ. Hold the model and task steady to isolate a context change.","단가와 작업 방식이 달라질 수 있습니다. 컨텍스트 변경의 효과를 보려면 모델과 작업을 맞추세요."],
    quality_regression:["Quality fell: review or restore","품질 저하: 검토하거나 원복하세요","Reported quality regressed. A lower bill does not compensate for an incorrect or unfinished result.","사용자가 품질 저하를 기록했습니다. 비용이 줄어도 틀리거나 미완성인 결과를 개선으로 보지 않습니다."],
    quality_unknown:["Task quality still needs a check","작업 품질 확인이 필요합니다","Check completion, tests and rework, then record satisfaction. A session or user turn is not automatically a completed task.","완료 여부·테스트·재작업을 확인한 뒤 만족도를 기록하세요. 세션이나 사용자 턴을 완료된 작업으로 자동 계산하지 않습니다."],
    coverage_gap:["Some usage records are missing","사용 기록 일부가 누락되었습니다","The totals may undercount work. Resolve coverage gaps before judging an optimization.","전체 작업량보다 적게 집계됐을 수 있습니다. 기록 누락을 확인한 뒤 최적화를 판단하세요."]
  };
  function comparisonHTML(a,b,options={}){
    const {t,n,money,pct}=format(options.language),r=compare(a,b,options),x=r.before,y=r.after;
    const beforeLabel=esc(options.beforeLabel||t("Before","전")),afterLabel=esc(options.afterLabel||t("After","후"));
    const rows=[
      [t("Total input","전체 입력"),x.input,y.input,n],
      [t("Model responses","모델 응답 횟수"),x.requests,y.requests,n],
      [t("Input / response","응답당 입력"),x.average,y.average,n],
      [t("Estimated total cost","추정 총비용"),x.cost,y.cost,money]
    ];
    return `<section class="economics" data-no-i18n><h3>${t("Did the whole workload get cheaper?","전체 작업에 든 비용도 줄었나요?")}</h3>
      <p>${t("Compare totals before celebrating a smaller prompt. These are observations, not matched-task savings.","프롬프트가 작아졌다면 전체 사용량도 확인하세요. 관측 비교이며, 동일 작업의 절감률은 아닙니다.")}</p>
      <div class="econ-pairs">${rows.map(([label,old,next,fmt])=>{
        const observed=x.requests>0&&y.requests>0&&old!==null&&next!==null,max=Math.max(old||0,next||0,1);
        return `<div class="econ-pair"><strong>${label}</strong><span>${observed?esc(pct(delta(old,next))):"—"}</span><div class="econ-pair-track"><i style="width:${observed?old/max*100:0}%"></i></div><small>${beforeLabel}: ${x.requests&&old!==null?esc(fmt(old)):"—"}</small><div class="econ-pair-track after"><i style="width:${observed?next/max*100:0}%"></i></div><small>${afterLabel}: ${y.requests&&next!==null?esc(fmt(next)):"—"}</small></div>`;
      }).join("")}</div>
      <div class="econ-signals">${r.signals.map(code=>{const m=messages[code];return `<div class="econ-signal ${code==="quality_regression"?"econ-danger":""}"><strong>${t(m[0],m[1])}</strong><p>${t(m[2],m[3])}</p></div>`;}).join("")}</div>
      <p class="small">${t("Decision: compare the same work at the same quality, including all recorded agents. Keep or restore based on those results. No completed-task count or productivity score is inferred.","판단 기준: 같은 작업·같은 품질에서 기록된 모든 에이전트를 합산하세요. 결과를 보고 유지 또는 원복하세요. 완료 작업 수나 생산성 점수는 추정하지 않습니다.")}</p></section>`;
  }
  function spendHTML(s,lang,groups=[]){
    const {t,n,money}=format(lang),m=measure(s);
    let spend=s?.spend;
    // Older saved reports and synthetic sessions carry price groups instead.
    if(!spend&&groups.length){
      spend={responses:0,usd:Object.fromEntries(keys.map(k=>[k,0]))};
      for(const g of groups){
        if(!g.rates||g.unpriced!==0||!keys.every(k=>Number.isFinite(g.usd?.[k])&&g.usd[k]>=0))continue;
        spend.responses+=safe(g.requests);
        for(const k of keys)spend.usd[k]+=g.usd[k];
      }
    }
    const detail=spend?.responses>0&&spend.responses<=m.requests&&keys.every(k=>Number.isFinite(spend.usd?.[k])&&spend.usd[k]>=0);
    const costTotal=detail?keys.reduce((sum,k)=>sum+spend.usd[k],0):0;
    const parts=[["cached_input","Cache reads","캐시 읽기","#278367"],["input","New input","새 입력","#386ed2"],["writes","Cache writes","캐시 쓰기","#bd7c19"],["output","Output","출력","#8061b4"]];
    const amount=k=>k==="writes"?safe(spend.usd.cache_write)+safe(spend.usd.cache_write_1h):spend.usd[k];
    return `<section class="economics" data-no-i18n><h3>${t("Cache reuse is useful. Check the total bill too.","캐시를 잘 써도, 총비용은 확인해야 합니다.")}</h3>
      <div class="econ-pairs"><div><span>${t("Input tokens served from cache","입력 토큰 중 캐시 읽기")}</span><strong class="econ-number">${m.cache===null?"—":n(m.cache*100)+"%"}</strong><small>${t("Token share, not request hit rate","토큰 비중 · 요청 적중률 아님")}</small></div><div><span>${t("Input processed across requests","반복 처리를 포함한 전체 입력")}</span><strong class="econ-number">${n(m.input)}</strong><small>${n(m.requests)} ${t("recorded responses","관측 응답")}</small></div><div><span>${t("Estimated total token cost","전체 토큰 추정 비용")}</span><strong class="econ-number">${m.cost===null?"—":money(m.cost)}</strong><small>${t("Includes output; not your subscription bill","출력 포함 · 구독 청구액 아님")}</small></div></div>
      ${detail?`<p class="small">${t("Cost breakdown coverage","비용 구성 산정 범위")}: ${n(spend.responses)} / ${n(m.requests)} ${t("responses","응답")}${spend.responses<m.requests?" · "+t("Partial amounts only","일부 금액만 산정"):""}</p><div class="econ-stack" aria-hidden="true">${parts.map(([k,en,ko,color])=>`<i style="width:${costTotal?amount(k)/costTotal*100:0}%;background:${color}"></i>`).join("")}</div><div class="econ-costs">${parts.map(([k,en,ko,color])=>`<div><i style="background:${color}" aria-hidden="true"></i>${t(en,ko)} <b>${money(amount(k))}</b></div>`).join("")}</div>`:`<p class="small">${t("A complete category cost breakdown is unavailable. No cache discount or saving is invented.","범주별 비용 근거가 없습니다. 캐시 할인액이나 절감액을 추정해 넣지 않습니다.")}</p>`}
      <p class="small">${t("Caching reuses input computation. It does not shrink the context window or remove extra calls. High cache use alone proves neither waste nor efficiency.","캐싱은 입력 계산을 재사용합니다. 컨텍스트 공간이나 추가 호출 자체를 줄이지는 않습니다. 캐시 비중만으로 낭비나 효율을 판정하지 않습니다.")}</p></section>`;
  }
  function timelineHTML(hours,lang){
    const {t,n}=format(lang);
    if(!hours?.some(h=>h.requests))return `<p class="econ-empty">${t("The cumulative graph appears after usage is recorded. Empty hours are not savings.","사용 기록이 생기면 누적 그래프가 나타납니다. 빈 시간은 절감이 아닙니다.")}</p>`;
    let total=0,cache=0,calls=0;
    const values=hours.map((h,i)=>{total+=input(h);cache+=safe(h.tokens?.cached_input);calls+=safe(h.requests);return {hour:i+1,input:total,cache,calls,requests:safe(h.requests)};});
    const max=Math.max(total,1),count=Math.max(values.length,1),x=i=>40+i/count*700,y=v=>175-v/max*140;
    const points=k=>`40,175 `+values.map((v,i)=>`${x(i+1)},${y(v[k])}`).join(" ");
    return `<section class="economics" data-no-i18n><h3>${t("The running total, including cache reads","캐시 읽기도 합산한 누적 사용량")}</h3>
      <p class="small">${t("Recorded input across requests, not occupied context or a 1M capacity gauge. Flat sections mean no additional recorded input. Hour labels identify buckets; the last bucket may be partial.","요청 전체의 처리량입니다. 컨텍스트 점유량이나 1M 용량 게이지가 아닙니다. 평평한 구간은 추가로 기록된 입력이 없다는 뜻입니다. 시간 표시는 집계 구간 번호이며, 마지막 구간은 일부 시간만 포함할 수 있습니다.")}</p>
      <div class="econ-legend"><span>● ${t("Total input","전체 입력")}</span><span>● ${t("Cache reads (part of total)","캐시 읽기 (전체에 포함)")}</span></div>
      <svg class="econ-chart" viewBox="0 0 800 215" role="img" aria-label="${t("Cumulative input and cache reads; exact values in the table below","누적 전체 입력과 캐시 읽기. 정확한 값은 아래 표에서 확인")}"><text x="40" y="20">${n(total)}</text><line x1="40" y1="175" x2="740" y2="175" stroke="#a8b3c6"/><polyline fill="none" stroke="#386ed2" stroke-width="3" points="${points("input")}"/><polyline fill="none" stroke="#278367" stroke-width="3" stroke-dasharray="6 4" points="${points("cache")}"/><text x="40" y="200">0h</text><text x="720" y="200">${values.length}h</text></svg>
      <details><summary>${t("Read cumulative values","누적 값 표로 보기")}</summary><div class="table-wrap"><table><thead><tr><th>${t("Trial hour","실험 시간")}</th><th>${t("Responses this hour","이번 시간 응답")}</th><th>${t("Total responses","누적 응답")}</th><th>${t("Total input","누적 전체 입력")}</th><th>${t("Cache reads","누적 캐시 읽기")}</th></tr></thead><tbody>${values.map(v=>`<tr><td>${v.hour}h</td><td>${n(v.requests)}</td><td>${n(v.calls)}</td><td>${n(v.input)}</td><td>${n(v.cache)}</td></tr>`).join("")}</tbody></table></div></details></section>`;
  }
  root.TraceEconomics={input,measure,compare,agents,incomplete,spendHTML,comparisonHTML,timelineHTML};
})(globalThis);
