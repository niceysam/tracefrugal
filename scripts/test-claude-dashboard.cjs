// Exercise the UI against a real Go import of a synthetic Claude Code log.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const vm = require("node:vm");
const {execFileSync} = require("node:child_process");
const elements = new Map();
function element(id) {
  if(!elements.has(id)) elements.set(id,{
    textContent:id==="bootstrap"?'{"mode":"claude-demo"}':"",innerHTML:"",hidden:false,
    classList:{toggle(){}},addEventListener(){},setAttribute(){},removeAttribute(){},
    showModal(){this.open=true},close(){this.open=false},
  });
  return elements.get(id);
}
const context = vm.createContext({document:{getElementById:element,querySelectorAll:()=>[],addEventListener(){}},setInterval(){},URLSearchParams,console});
vm.runInContext(fs.readFileSync("internal/webui/economics.js","utf8"),context);
vm.runInContext(fs.readFileSync("internal/webui/claude.js","utf8"),context);
const run=code=>vm.runInContext(code,context);
async function test() {
  await run("refresh()");
  assert.equal(run("state.sessions.length"),7);
  assert.ok(element("chart").innerHTML.includes("<svg"));
  assert.ok(element("chart").innerHTML.includes("<rect"));
  const sum=run("state.summary.requests");
  assert.equal(run("state.sessions.reduce((n,s)=>n+s.requests,0)"),sum);
  run('recipeID="mcp"');
  await run('change("try","",4)');
  assert.equal(run("state.changes[0].status"),"saved");
  assert.equal(run("state.changes[0].hours.length"),24);
  assert.ok(element("diagnosis").innerHTML.includes("lower-priced"));
  assert.ok(element("tool-sources").innerHTML.includes("not token attribution"));
  await run('change("rate",state.changes[0].id,2)');
  assert.ok(element("changes-list").innerHTML.includes("satisfaction fell"));
  assert.ok(element("changes-list").innerHTML.includes("Input / user turn"));
  run("restoreID=state.changes[0].id");
  await run('change("restore")');
  assert.equal(run("state.changes[0].status"),"restored");
  await run("refresh()");
  assert.equal(run("state.changes.length"),1);
  const root=fs.mkdtempSync(path.join(os.tmpdir(),"tracefrugal-ui-"));
  try {
    const dir=path.join(root,"projects","synthetic");
    fs.mkdirSync(dir,{recursive:true});
    const row={type:"assistant",timestamp:new Date(Date.now()-60000).toISOString(),sessionId:"synthetic",cwd:"/demo/<b>untrusted</b>",
      message:{id:"msg_test",model:"claude-sonnet-4-6",content:"PRIVATE_CONTENT_SENTINEL",usage:{input_tokens:100,output_tokens:25,cache_read_input_tokens:1000}}};
    fs.writeFileSync(path.join(dir,"session.jsonl"),JSON.stringify(row)+"\n"+JSON.stringify(row)+"\n");
    context.native=JSON.parse(execFileSync("go",["run","./cmd/tracefrugal","claude","--dir",root,"--json"],{encoding:"utf8"}));
    assert.equal(context.native.summary.requests,1);
    assert.equal(context.native.sessions[0].requests,1);
    assert.equal(context.native.sessions[0].timeline.length,1);
    run("state=native; render()");
    assert.ok(element("metrics").innerHTML.includes("1 responses"));
    assert.ok(!element("sessions").innerHTML.includes("<b>"));
    assert.ok(!JSON.stringify(context.native).includes("PRIVATE_CONTENT_SENTINEL"));
    run("selected=state.sessions[0].id; render()");
    assert.ok(element("chart-title").textContent.includes("request by request"));
    assert.ok(element("chart").innerHTML.includes("Back to all sessions"));
    // Unknown cost is a gap, never an apparently free request.
    run("state.sessions[0].timeline[0].cost=null; unit='cost'; renderChart()");
    assert.ok(element("chart").innerHTML.includes("not zero cost"));
    assert.ok(!element("chart").innerHTML.includes("<rect"));
    run("hideNames=true; renderSessions()");
    assert.ok(!element("sessions").innerHTML.includes("untrusted"));
  } finally { fs.rmSync(root,{recursive:true,force:true}); }
  console.log("Claude dashboard: real Go import, totals, chart, price gaps, privacy, trial, history and undo passed.");
}
test().catch(e=>{console.error(e);process.exitCode=1});
