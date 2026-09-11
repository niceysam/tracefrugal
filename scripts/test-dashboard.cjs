// Test state transitions and import boundaries without a browser dependency.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const {execFileSync} = require("node:child_process");
const elements = new Map();
function element(id) {
  if (!elements.has(id)) elements.set(id, {
    textContent: id === "bootstrap" ? '{"mode":"demo"}' : "", innerHTML:"", hidden:false,
    classList:{toggle(){},contains(){return false}}, addEventListener(){}, setAttribute(){},
    showModal(){this.open=true},close(){this.open=false}
  });
  return elements.get(id);
}
const context = vm.createContext({
  document:{getElementById:element, querySelector:element,querySelectorAll:()=>[],addEventListener(){}},
  setTimeout: f => f(), setInterval(){}, URLSearchParams, console,
});
vm.runInContext(fs.readFileSync("internal/webui/dashboard.js","utf8"),context);
const run = code => vm.runInContext(code, context);
async function test() {
  assert.equal(run("state.active_cap"),512);
  assert.equal(run("state.current_report.successful_tasks"),2);
  assert.ok(Math.abs(run("state.spend")-.06008)<1e-10);
  await run('act("try")');
  assert.equal(run("state.active_cap"),256);
  await run('act("try")');
  assert.equal(run("state.entries[0].status"),"rejected");
  assert.equal(run("state.active_cap"),256);
  assert.equal(run("state.current_report.successful_tasks"),2);
  const spend=run("state.spend");
  assert.ok(Math.abs(spend-.12264)<1e-10);
  assert.ok(run('historyPoints(state.entries).some(p=>p.status==="rejected")'));
  await run('act("rollback")');
  assert.equal(run("state.active_cap"),512);
  assert.equal(run("state.paused"),true);
  assert.equal(run("state.spend"),spend);
  await run('act("resume")');
  assert.equal(run("state.paused"),false);
  assert.equal(run("state.entries.length"),5);
  assert.equal(run('validateReport(demoReport(512)).requests'),2);
  assert.throws(()=>run('validateReport({usage:{input_tokens:100}})'));
  assert.throws(()=>run('validateReport({...demoReport(512), estimated_cost_usd:0})'));
  assert.throws(()=>run('validateReport({...demoReport(512), estimated_cost_per_success_usd:0})'));
  assert.throws(()=>run('validateReport({...demoReport(512), tokens:{...demoReport(512).tokens, input:-1}})'));
  assert.equal(run('esc("<img src=x onerror=alert(1)>")'),"&lt;img src=x onerror=alert(1)&gt;");
  // Consume a real CLI export so the viewer and Go report schema cannot drift.
  context.exportedReport = JSON.parse(execFileSync("go",["run","./cmd/tracefrugal","report",
    "--trace","examples/baseline.jsonl","--prices","examples/prices.json","--format","json"],{encoding:"utf8"}));
  assert.ok(Math.abs(run("validateReport(exportedReport).estimated_cost_usd")-.66)<1e-12);
  run("state={report:validateReport(exportedReport),entries:[],spend:0}; render()");
  assert.ok(element("metrics").innerHTML.includes("$0.6600"));
  assert.equal((element("trend").innerHTML.match(/<rect /g)||[]).length,2);
  console.log("Dashboard: apply, reject, rollback, resume, costs, import validation passed.");
}
test().catch(error=>{console.error(error);process.exitCode=1});
