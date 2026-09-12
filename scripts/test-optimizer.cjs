const assert=require("node:assert/strict"),fs=require("node:fs"),vm=require("node:vm");
const elements=new Map(),calls=[];
function element(id){if(!elements.has(id))elements.set(id,{value:"",textContent:"",innerHTML:"",classList:{toggle(){}},addEventListener(){}});return elements.get(id);}
element("optimizer-bootstrap").textContent='{"token":"local-token"}';
const sample={requests:0,tokens:{},models:[],unpriced:0,known_usd:0};
const report={diagnosis:{ready:true,plan:"preview",version:"2.1.268",checked:"2026-09-12T00:00:00Z",documents:[],project:"<private>",binary:"claude",current:"default"},recent:sample,trials:[],health:{}};
const ctx=vm.createContext({console,URL,URLSearchParams,Intl,Date,navigator:{language:"ko-KR"},location:{href:"http://localhost?lang=ko"},localStorage:{getItem(){return null},setItem(){}},setInterval(){},
 document:{documentElement:{},getElementById:element,querySelectorAll(){return []},addEventListener(){}},
 fetch:async(url,options)=>{calls.push({url,options});return {ok:true,json:async()=>structuredClone(report)}}});
const source=fs.readFileSync("internal/webui/optimizer.js","utf8");
assert.ok(!source.includes("\uFFFD"),"Korean replacement character");
vm.runInContext(fs.readFileSync("internal/webui/economics.js","utf8"),ctx);
vm.runInContext(source,ctx);
async function run(){
 await vm.runInContext("refresh()",ctx);
 assert.match(element("intro").innerHTML,/원복/);
 assert.match(element("diagnosis").innerHTML,/&lt;private&gt;/);
 assert.match(element("preview").innerHTML,/6000/);
 assert.match(vm.runInContext("chart({hours:[],baseline:{requests:0}})",ctx),/빈 시간은 절감이 아닙니다/);
 await vm.runInContext("act('apply')",ctx);
 const apply=calls.find(c=>c.url==="api/apply");
 assert.equal(apply.options.body.get("token"),"local-token");
 assert.equal(apply.options.body.get("rating"),"0","do not invent satisfaction");
 assert.equal(apply.options.body.get("plan"),"preview");
 const trial={id:"trial",status:"active",started:"2026-09-12T00:00:00Z",events:[],receipts:[],baseline:sample,after:sample,hours:[],assessment:"Waiting for a Claude SessionStart receipt with the proposed setting.",before_rating:0,after_rating:0,can_restore:true};
 report.trials=[trial];
 await vm.runInContext("refresh()",ctx);
 assert.match(element("preview").innerHTML,/이제 새 Claude 세션/);
 assert.match(element("history").innerHTML,/— → —/);
 assert.ok(!element("history").innerHTML.includes("0/5"));
 element("rate-trial").value="4";
 await vm.runInContext("act('keep','trial')",ctx);
 assert.equal(calls.at(-1).url,"api/rate");
 assert.equal(calls.at(-1).options.body.get("outcome"),"keep");
 await vm.runInContext("act('restore','trial')",ctx);
 assert.equal(calls.at(-1).options.body.get("id"),"trial");
 report.trials[0].status="restored";
 await vm.runInContext("refresh()",ctx);
 assert.match(element("preview").innerHTML,/새 실험으로 다시 적용/);
 report.log_error=true;
 await vm.runInContext("refresh()",ctx);
 assert.match(element("diagnosis").innerHTML,/기록이 불완전/);
 vm.runInContext("language='en';render()",ctx);
 assert.match(element("preview").innerHTML,/Reapply as a new trial/);
 console.log("Optimizer: EN/KO, escaped paths, empty evidence, unknown quality, CSRF-bound apply, restore/reapply and missing-log visibility passed.");
}
run().catch(e=>{console.error(e);process.exitCode=1;});
