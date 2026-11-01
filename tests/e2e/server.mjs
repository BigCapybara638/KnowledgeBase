import {mkdtempSync,rmSync,mkdirSync} from 'node:fs'
import {tmpdir} from 'node:os'
import {join} from 'node:path'
import {spawn,spawnSync} from 'node:child_process'
const dir=mkdtempSync(join(tmpdir(),'kb-e2e-'))
mkdirSync('bin',{recursive:true})
for(const args of [['build','-o','bin/kb','./cmd/kb'],['run','./tests/e2e/seed',dir]]){
 const r=spawnSync('go',args,{stdio:'inherit'});if(r.status!==0){rmSync(dir,{recursive:true,force:true});process.exit(r.status??1)}
}
const child=spawn('./bin/kb',['serve'],{stdio:'inherit',env:{...process.env,KB_DATA_DIR:dir,KB_DEV:'true',KB_PUBLIC_URL:'http://127.0.0.1:4173',KB_LISTEN_ADDR:'127.0.0.1:4173'}})
for(const signal of ['SIGTERM','SIGINT'])process.on(signal,()=>child.kill('SIGTERM'))
child.on('exit',code=>{rmSync(dir,{recursive:true,force:true});process.exit(code??0)})
