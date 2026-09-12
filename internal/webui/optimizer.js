"use strict";
const boot=JSON.parse(document.getElementById("optimizer-bootstrap").textContent);
const $=id=>document.getElementById(id);
const esc=v=>String(v??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
let language=new URL(location.href).searchParams.get("lang"), state=null, busy=false;
try{language=language||localStorage.getItem("tracefrugal-language");}catch{}
if(!["en","ko"].includes(language))language=navigator.language.startsWith("ko")?"ko":"en";
const t=(en,ko)=>language==="ko"?ko:en;
const number=n=>new Intl.NumberFormat(language==="ko"?"ko-KR":"en-US",{maximumFractionDigits:1}).format(n||0);
const input=s=>(s?.tokens?.input||0)+(s?.tokens?.cached_input||0)+(s?.tokens?.cache_write||0)+(s?.tokens?.cache_write_1h||0);
const avg=s=>s?.requests?input(s)/s.requests:null;
const stamp=x=>new Date(x).toLocaleString(language==="ko"?"ko-KR":"en-US");
const money=s=>!s?.requests||s.unpriced? t("Unpriced / incomplete","금액 미산정 / 불완전"):"$"+Number(s.known_usd).toFixed(4);
const notice=(s,error=false)=>{$("notice").textContent=s;$("notice").classList.toggle("error",error);};
const message=v=>{
 const map={
  "Preview one project-local change. Start a fresh Claude session after applying.":"프로젝트 설정 변경을 미리 확인하세요. 적용한 뒤 새 Claude 세션을 시작하세요.",
  "This project already has the proposed limit. No additional saving is assumed.":"이미 제안한 제한이 설정되어 있습니다. 추가 절감을 가정하지 않습니다.",
  "An equal or smaller limit is already configured. No optimization is proposed.":"같거나 더 작은 제한이 이미 설정되어 있어 변경을 제안하지 않습니다.",
  "Original settings restored. This history is retained; start a new trial to compare again.":"기존 설정으로 원복했습니다. 이력은 남습니다. 다시 비교하려면 새 실험을 시작하세요.",
  "Official documentation could not be verified. Retry diagnosis with internet access.":"공식 문서를 확인하지 못했습니다. 인터넷 연결 후 다시 진단하세요.",
  "Collect comparable work and rate its quality. This is an observational comparison, not causal savings.":"비슷한 작업을 수행하고 품질을 평가하세요. 사용 전후 관측 비교이며, 절감의 인과관계를 입증하지 않습니다.",
  "Waiting for a Claude SessionStart receipt with the proposed setting.":"제안한 설정값으로 시작한 Claude 세션을 기다립니다.",
  "Session setting observed. Waiting for usage from that session.":"새 세션의 설정값을 확인했습니다. 해당 세션의 사용량을 기다립니다.",
  "Quality regression reported. Restore before continuing.":"품질 저하가 보고되었습니다. 계속하기 전에 원복을 권합니다.",
  "Usage is available. Rate quality before deciding whether to keep the change.":"사용량이 집계되었습니다. 품질을 평가한 뒤 유지 여부를 결정하세요.",
  "Usage observed, but no prior project baseline. No saving can be calculated.":"사용량은 관측됐지만 프로젝트의 이전 기준 데이터가 없습니다. 절감률을 계산할 수 없습니다.",
  "Settings changed outside TraceFrugal. Automatic restore is blocked; review the local backup.":"외부에서 설정이 수정됐습니다. 편집을 보호하기 위해 자동 원복을 차단했습니다. 로컬 백업을 확인하세요.",
  "Interrupted change. Use restore to recover the original settings.":"변경이 중단되었습니다. 원복을 눌러 기존 설정을 복구하세요."
 };
 if(language==="ko"&&v?.startsWith("This recipe was verified on Claude Code"))return "이 최적화는 Claude Code 2.1.268에서 검증했습니다. 현재 버전은 호환성 확인이 필요하여 적용하지 않습니다.";
 return language==="ko"?(map[v]||v):v;
};
function rating(id,value){
 return `<select id="${esc(id)}"><option value="">${t("Choose 1–5","1–5점 선택")}</option>${[1,2,3,4,5].map(n=>`<option value="${n}" ${n===value?"selected":""}>${n} · ${t(["Very poor","Poor","Mixed","Good","Very good"][n-1],["매우 불만족","불만족","보통","만족","매우 만족"][n-1])}</option>`).join("")}</select>`;
}
function chart(v){
 const populated=v.hours.some(h=>h.requests);
 if(!populated)return `<div class="chart-empty">${t("Your hourly graph appears after a new, verified Claude session records usage. Empty hours are not savings.","설정이 확인된 새 Claude 세션의 사용량이 기록되면 시간별 그래프가 나타납니다. 빈 시간은 절감이 아닙니다.")}</div>`;
 const base=avg(v.baseline)||0, max=Math.max(base,...v.hours.map(h=>avg(h)||0),1), y=n=>160-n/max*130;
 let svg=`<svg viewBox="0 0 800 205" role="img" aria-label="${t("Input tokens per response, by trial hour","실험 시간별 응답당 입력 토큰")}"><line x1="45" y1="${y(base)}" x2="785" y2="${y(base)}" stroke="#b1832d" stroke-dasharray="5 4"/><text x="45" y="16" font-size="11" fill="#536078">${esc(number(max))}</text>`;
 v.hours.forEach((h,i)=>{const x=50+i*30; if(h.requests)svg+=`<rect x="${x}" y="${y(avg(h))}" width="20" height="${160-y(avg(h))}" rx="3" fill="#356ac5"><title>${i+1}h: ${number(avg(h))}</title></rect>`;svg+=`<text x="${x}" y="181" font-size="10" fill="#536078">${i+1}h</text>`;});
 return `<div class="legend"><span><i class="dot" style="background:#b1832d"></i>${t("Previous day average","전날 평균")}</span><span><i class="dot" style="background:#356ac5"></i>${t("Verified sessions after applying","적용 확인된 세션")}</span></div>${svg}</svg>`;
}
function render(){
 if(!state)return;
 const drafts=[...document.querySelectorAll("select")].filter(x=>x.id!=="language").map(x=>[x.id,x.value]);
 document.documentElement.lang=language;$("language").value=language;
 document.title=t("TraceFrugal — Optimize your local Claude","TraceFrugal — 내 Claude 최적화");
 const d=state.diagnosis, active=state.trials.find(v=>v.status!=="restored"), latest=state.trials.at(-1);
 const verified=active?.receipts.some(r=>r.limit==="6000");
 $("intro").innerHTML=`<span class="eyebrow">${t("LOCAL CLAUDE CODE OPTIMIZER","내 컴퓨터의 CLAUDE CODE 최적화")}</span><h1>${t("Less context. Check the result.<br>Keep it or go back.","컨텍스트를 줄이고, 결과를 확인하세요.<br>좋으면 유지하고, 아니면 원복하세요.")}</h1><p>${t("This app runs on your computer. The browser is its local control panel. Your settings and usage stay here.","이 프로그램은 내 컴퓨터에서 실행됩니다. 브라우저는 로컬 조작 화면이며, 설정과 사용 기록은 이 컴퓨터에 남습니다.")}</p>`;
 $("steps").innerHTML=[t("1 · Check your CLI","1 · CLI 확인"),t("2 · Apply one change","2 · 최적화 적용"),t("3 · Use & compare","3 · 사용하며 비교"),t("4 · Keep or restore","4 · 유지 / 원복")].map((x,i)=>`<span class="${(active?(active.events.some(e=>e.action==="kept")?3:verified?2:1):latest?3:0)===i?"current":""}">${x}</span>`).join("");
 $("diagnosis").innerHTML=`<article class="panel"><span class="pill">Claude Code ${esc(d.version||t("not detected","미확인"))}</span><h2>${t("Your environment, checked first","내 환경부터 확인했습니다")}</h2><div class="grid"><div><h3>${t("Selected CLI","사용할 CLI")}</h3><p class="path">${esc(d.binary)}</p><h3>${t("This project only","적용할 프로젝트")}</h3><p class="path">${esc(d.project)}</p></div><div><h3>${t("Official documentation","공식 문서 확인")}</h3><p>${esc(stamp(d.checked))} · ${d.documents?.length||0}/4</p><p>${t("A compiled compatibility recipe, checked against official guidance. Unsupported versions stay read-only.","공식 지침과 대조한 호환성 규칙을 사용합니다. 검증하지 않은 버전은 조회만 가능합니다.")}</p><button data-action="diagnose" ${busy?"disabled":""}>${t("Check again","다시 진단")}</button></div></div><details><summary>${t("Sources and exact setting","공식 근거와 설정 경로")}</summary>${(d.documents||[]).map(doc=>`<p><a href="${esc(doc.url)}" target="_blank" rel="noreferrer">${esc(doc.url)}</a></p>`).join("")}<p class="path">${esc(d.target)}</p><p>${t("Tool search is left as configured. It is already deferred by default in the reviewed CLI. Model, reasoning effort, permissions and MCP connections are not changed.","도구 검색은 기존 설정을 유지합니다. 검증한 CLI에서는 이미 지연 로딩이 기본입니다. 모델·추론 강도·권한·MCP 연결은 변경하지 않습니다.")}</p></details></article>`;
 $("preview").innerHTML=active
 ? `<article class="panel"><span class="eyebrow">${t("ONE CHANGE IS ACTIVE","최적화 1건 적용 중")}</span><h2>${verified?t("Claude started with the new setting.","새 Claude 세션에서 설정값을 확인했습니다."):t("Saved. Now start a fresh Claude session.","저장했습니다. 이제 새 Claude 세션을 시작하세요.")}</h2><p>${t("Settings stay active until you restore them. Close and reopen this panel without losing history.","원복할 때까지 설정은 유지됩니다. 화면을 닫았다 다시 열어도 이력은 남습니다.")}</p><p>${t("Open your usual terminal in this project and start the selected Claude CLI. Existing sessions keep their context and may keep their prior settings.","평소 쓰는 터미널에서 이 프로젝트를 열고 위에 표시된 Claude CLI로 새 세션을 시작하세요. 기존 세션의 컨텍스트와 설정은 그대로일 수 있습니다.")}</p><p class="callout">${esc(message(active.assessment))}</p><p>${t("A SessionStart receipt confirms the setting in that process. It does not prove the provider charged fewer tokens.","세션 시작 기록은 해당 프로세스의 설정값을 확인합니다. 이것만으로 토큰 비용 절감이 입증되지는 않습니다.")}</p><button class="danger" data-action="restore" data-id="${esc(active.id)}" ${!active.can_restore||busy?"disabled":""}>${t("Restore original settings","기존 설정으로 원복")}</button></article>`
 : `<article class="panel"><span class="eyebrow">${t("RECOMMENDED TRIAL · NO EXTRA LLM","추천 실험 · 추가 LLM 사용 없음")}</span><h2>${t("Read large tool results only when needed.","큰 도구 결과는 필요할 때 읽게 하세요.")}</h2><div class="flow"><div>${t("MCP returns a large result","MCP가 큰 결과를 반환")}</div><b>→</b><div>${t("Claude saves it to a local file","Claude가 로컬 파일에 보관")}</div><b>→</b><div>${t("The agent reads relevant sections","에이전트가 필요한 부분을 읽음")}</div></div><p>${t("Lower Claude’s native MCP result threshold to 6,000 tokens. Eligible large text results become file references. Tools with their own size override are exempt; images follow the CLI’s limits.","Claude의 MCP 결과 기준을 6,000토큰으로 낮춥니다. 해당 기준을 넘는 텍스트 결과는 파일 참조로 바뀝니다. 자체 크기 제한이 있는 도구는 예외이며, 이미지는 CLI 제한을 따릅니다.")}</p><div class="grid"><div><h3>${t("Exact change","실제 변경 내용")}</h3><pre>env.MAX_MCP_OUTPUT_TOKENS\n${esc(d.current)} → 6000</pre><p class="muted">${t("Adds a local SessionStart receipt hook. It writes hashed session identity and this setting only; no prompt text. Your original file is backed up before changing it.","세션 시작 확인용 로컬 훅도 추가합니다. 해시된 세션 번호와 이 설정값만 기록하고 프롬프트 본문은 기록하지 않습니다. 원본 설정 파일은 변경 전에 백업합니다.")}</p></div><div><h3>${t("Before you try","시험하기 전에")}</h3><label class="rating" for="before-rating">${t("How useful are answers today? (optional)","지금 답변과 추론에 얼마나 만족하나요? (선택)")}</label>${rating("before-rating",0)}<p class="muted">${t("Extra file reads can add calls or miss context. Compare task quality as well as input tokens. Keep ordinary permissions and inspect the proposed scope.","추가 파일 읽기가 호출 수를 늘리거나 맥락을 놓칠 수 있습니다. 입력량과 함께 작업 품질도 비교하세요.")}</p></div></div><p class="callout ${d.ready?"":"warning"}">${esc(message(d.reason))}</p><button class="primary" data-action="apply" ${!d.ready||busy?"disabled":""}>${latest?t("Reapply as a new trial","새 실험으로 다시 적용"):t("Apply optimization · save backup","최적화 적용 · 원본 백업")}</button><p class="muted">${t("A 24-hour observation window. Settings remain until you restore them. No automatic model calls or paid benchmark runs.","24시간 관측합니다. 설정은 원복할 때까지 유지됩니다. 모델 호출이나 유료 벤치마크는 자동 실행하지 않습니다.")}</p></article>`;
 $("history").innerHTML=`<h2>${t("Your results and history","전후 비교와 변경 이력")}</h2>`+(state.trials.length?state.trials.slice().reverse().map(trialHTML).join(""):`<article class="panel"><h3>${t("Start with your real baseline","실제 사용 기록을 기준으로 시작하세요")}</h3><p>${t("Previous 24 hours in this exact project","이 프로젝트의 최근 24시간")} · ${number(state.recent.requests)} ${t("model responses","모델 응답")}</p><p>${t("After applying, only sessions with a matching activation receipt enter the comparison. Nothing is reported as saved before usage arrives.","적용 후에는 설정이 확인된 세션만 비교에 포함합니다. 사용 기록이 생기기 전에는 절감했다고 표시하지 않습니다.")}</p></article>`);
 $("footer").textContent=t("Official-doc checks contact public documentation sites. No project data is sent. Native usage is observational, prices are estimates, and incomplete coverage stays visible. A lower cost is not proof of preserved quality.","공식 문서 확인 시 공개 문서 사이트에 접속하며 프로젝트 데이터는 보내지 않습니다. 사용량은 관측값이고 금액은 추정치입니다. 비용이 줄었다고 품질이 유지됐다는 뜻은 아닙니다.");
 const health=state.health||{};
 if(state.log_error||health.missing||health.invalid||health.partial||health.unreadable||health.conflicts)$("diagnosis").innerHTML+=`<p class="callout warning">${t("Usage coverage is incomplete. Missing or unreadable records are not zero usage. Settings can still be restored.","사용 기록이 불완전합니다. 없거나 읽지 못한 기록은 사용량 0이 아닙니다. 설정 원복은 가능합니다.")} ${t("Unreadable","읽기 실패")} ${number(health.unreadable)} · ${t("Invalid / partial","잘못된 기록 / 일부 기록")} ${number(health.invalid)} / ${number(health.partial)}</p>`;
 if(d.warnings?.length)$("diagnosis").innerHTML+=`<p class="callout warning">${t("A launch environment override was found. The session receipt must confirm the effective setting.","실행 환경의 설정 덮어쓰기가 감지됐습니다. 세션 시작 기록에서 실제 적용값을 확인해야 합니다.")}</p>`;
 drafts.forEach(([id,value])=>{if($(id))$(id).value=value;});
}
function trialHTML(v){
 const a=v.baseline,b=v.after;
 const delta=avg(a)&&avg(b)!==null?(avg(b)/avg(a)-1)*100:null;
 const comparison=(label,x,y)=>`<tr><td>${label}</td><td>${x}</td><td>${y}</td></tr>`;
 return `<article class="panel"><span class="pill">${v.status==="restored"?t("Restored","원복됨"):t("Trial","실험")} · ${esc(stamp(v.started))}</span><h2>${t("Did this change help?","이 변경이 도움이 됐나요?")}</h2><p class="callout ${v.outcome==="regression"?"warning":""}">${esc(message(v.assessment))}</p><div class="metrics"><div class="metric"><span>${t("Input / response change","응답당 입력 변화")}</span><strong>${delta===null?"—":(delta>0?"+":"")+number(delta)+"%"}</strong><small>${t("Observed, not attributed savings","관측 변화 · 절감 확정 아님")}</small></div><div class="metric"><span>${t("Recorded session starts","기록된 세션 시작")}</span><strong>${v.receipts.filter(r=>r.limit==="6000").length}</strong><small>${t("Matching effective setting","설정값 일치")}</small></div><div class="metric"><span>${t("Answer satisfaction","답변·추론 만족도")}</span><strong>${v.before_rating?v.before_rating+"/5":"—"} → ${v.after_rating?v.after_rating+"/5":"—"}</strong><small>${t("Your assessment","사용자 평가")}</small></div></div>${chart(v)}<div class="table-wrap"><table><thead><tr><th>${t("Measure","항목")}</th><th>${t("Previous 24 hours","적용 전 24시간")}</th><th>${t("Verified sessions, up to 24 hours","적용 확인 세션 · 최대 24시간")}</th></tr></thead><tbody>${comparison(t("Model responses","모델 응답 횟수"),number(a.requests),number(b.requests))}${comparison(t("Input / response","응답당 입력"),avg(a)===null?"—":number(avg(a)),avg(b)===null?"—":number(avg(b)))}${comparison(t("Total input tokens","전체 입력 토큰"),number(input(a)),number(input(b)))}${comparison(t("Cache-read tokens","캐시 읽기 토큰"),number(a.tokens?.cached_input),number(b.tokens?.cached_input))}${comparison(t("New input + cache writes","새 입력 + 캐시 쓰기"),number(input(a)-(a.tokens?.cached_input||0)),number(input(b)-(b.tokens?.cached_input||0)))}${comparison(t("Output tokens","출력 토큰"),number(a.tokens?.output),number(b.tokens?.output))}${comparison(t("Estimated token cost","토큰 기준 추정 비용"),money(a),money(b))}${comparison(t("Models","사용 모델"),esc((a.models||[]).join(", ")||"—"),esc((b.models||[]).join(", ")||"—"))}</tbody></table></div><p class="muted">${t("Different tasks, models, cache warmth and observation lengths affect this comparison. Include retries and lost quality when deciding. No task-matched savings claim is made.","작업·모델·캐시 상태·관측 기간이 다르면 결과도 달라집니다. 재작업과 품질 저하까지 고려해 판단하세요. 동일 작업을 통제한 절감률이 아닙니다.")}</p><div class="rating-row">${rating("rate-"+v.id,v.after_rating)}<select id="outcome-${esc(v.id)}"><option value="unknown">${t("Quality not checked","품질 확인 전")}</option><option value="good" ${v.outcome==="good"?"selected":""}>${t("Tasks still satisfactory","작업 결과 만족")}</option><option value="regression" ${v.outcome==="regression"?"selected":""}>${t("Worse results or more rework","결과 악화 / 재작업 증가")}</option></select><button data-action="rate" data-id="${esc(v.id)}" ${busy?"disabled":""}>${t("Save my assessment","평가 저장")}</button>${v.status==="active"?`<button class="primary" data-action="keep" data-id="${esc(v.id)}" ${busy||!v.after.requests?"disabled":""}>${t("Keep this setting","이 설정 계속 사용")}</button>`:""}</div><details><summary>${t("Change history and activation evidence","변경 이력과 적용 확인 기록")}</summary><ul>${v.events.map(e=>`<li>${esc(stamp(e.time))} · ${esc(t(e.action,({prepared:"변경 준비",applied:"설정 적용",restored:"원복",rated:"평가 저장",kept:"설정 유지 결정"})[e.action]||e.action))}</li>`).join("")}</ul><p class="muted">${t("Restore affects future sessions. It does not reconstruct earlier context, reverse tool actions or refund usage. Keep the TraceFrugal executable at the shown path while the receipt hook is installed.","원복은 이후 세션에 적용됩니다. 이전 컨텍스트·도구 작업·사용 비용은 되돌리지 않습니다. 확인 훅을 사용하는 동안 TraceFrugal 실행 파일을 같은 위치에 유지하세요.")}</p></details></article>`;
}
async function refresh(){
 if(busy)return;
 try{const r=await fetch("api/state");if(!r.ok)throw new Error(await r.text());state=await r.json();render();}catch(e){notice(e.message,true);}
}
async function act(action,id){
 if(busy)return;
 const fields={token:boot.token};
 if(action==="apply"){
  fields.rating=$("before-rating").value;fields.plan=state.diagnosis.plan;
  if(!fields.rating)fields.rating="0";
 }
 if(id)fields.id=id;
 if(action==="rate"||action==="keep"){
  fields.rating=$("rate-"+id).value;fields.outcome=action==="keep"?"keep":$("outcome-"+id).value;
  if(!fields.rating){notice(t("Choose a rating first.","만족도를 선택하세요."),true);return;}
 }
 busy=true;render();
 notice(t("Working locally…","이 컴퓨터에서 처리 중…"));
 try{
  const response=await fetch("api/"+(action==="keep"?"rate":action),{method:"POST",body:new URLSearchParams(fields)});
  if(!response.ok)throw new Error(await response.text());
  state=await response.json();
  notice(action==="restore"?t("Original settings restored. Start a fresh Claude session. History is retained.","기존 설정으로 원복했습니다. 새 Claude 세션을 시작하세요. 이력은 보존됩니다."):action==="apply"?t("Backed up and saved. Start a fresh Claude session to verify activation.","원본 백업 후 저장했습니다. 새 Claude 세션에서 적용 여부를 확인하세요."):t("Updated.","갱신했습니다."));
 }catch(e){notice(e.message,true);}finally{busy=false;render();}
}
document.addEventListener("click",event=>{const b=event.target.closest("[data-action]");if(b)act(b.dataset.action,b.dataset.id);});
$("language").addEventListener("change",()=>{language=$("language").value;try{localStorage.setItem("tracefrugal-language",language);}catch{}render();});
refresh();setInterval(refresh,30000);
