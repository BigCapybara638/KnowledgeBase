"""Run with --seconds 600 for acceptance; shorter runs are labelled smoke checks."""
import argparse
from pathlib import Path
import json
import os
import subprocess
import sys
import time
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'integration'))
from harness import Harness,ROOT

p=argparse.ArgumentParser();p.add_argument('--seconds',type=int,default=600);args=p.parse_args()
assert args.seconds>=10
output=ROOT/'.validation';output.mkdir(exist_ok=True)
with Harness() as h:
    seed=h.docker('run','--rm','-i','--network','none','--read-only','--tmpfs','/tmp:rw,noexec,nosuid,size=32m','--volumes-from',h.container(h.kb_project,'knowledgebase-app'),'--mount','type=bind,src='+str(ROOT/'bin/kb-load')+',dst=/tools/kb-load,readonly','--entrypoint','/tools/kb-load',h.env['KB_IMAGE'],'seed','--data-dir','/data',input=json.dumps({'password':h.password}).encode())
    data=json.loads(seed.stdout);assert data['articles']==5000
    proxy_peak=lambda: int(h.docker('exec',h.proxy_id,'cat','/sys/fs/cgroup/memory.peak').stdout)
    proxy_base=int(h.docker('exec',h.proxy_id,'cat','/sys/fs/cgroup/memory.current').stdout)
    image=json.loads(h.docker('image','inspect',h.env['KB_IMAGE']).stdout)[0]
    command=[str(ROOT/'bin/kb-load'),'run','--url',h.origin,'--ca',str(h.cert_dir/'cert.pem'),'--duration',str(args.seconds)+'s','--hot-id',data['hotId'],'--category-id',data['categoryId']]
    with (output/'capacity-worker.json').open('w') as out,(output/'capacity-worker.log').open('w') as err:
        worker=subprocess.Popen(command,stdin=subprocess.PIPE,stdout=out,stderr=err)
        worker.stdin.write(json.dumps({'password':h.password}).encode());worker.stdin.close()
        started=time.monotonic();backed_up=False
        try:
            while worker.poll() is None:
                if not backed_up and time.monotonic()-started>=min(30,args.seconds/3):
                    h.compose(h.kb_project,'exec','-T','knowledgebase-app','/app/kb','backup');backed_up=True
                time.sleep(2)
        finally:
            if worker.poll() is None:worker.terminate();worker.wait(timeout=20)
    result=json.loads((output/'capacity-worker.json').read_text())
    metrics=json.loads(h.compose(h.kb_project,'exec','-T','knowledgebase-app','/app/kb','metrics').stdout)
    ingress=max(0,proxy_peak()-proxy_base)
    result.update(app=metrics,proxyBaselineBytes=proxy_base,proxyIncrementalPeakBytes=ingress,totalAddedPeakBytes=metrics['peakBytes']+ingress,backupDuringLoad=backed_up,acceptanceRun=args.seconds>=600,imageID=image['Id'],imageSizeBytes=image['Size'],gitCommit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),timestamp=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),hostCPUs=os.cpu_count())
    (output/'capacity.json').write_text(json.dumps(result,indent=2,ensure_ascii=False)+'\n')
    print(json.dumps(result,indent=2,ensure_ascii=False),flush=True)
    assert worker.returncode==0,'load worker failed; see .validation/capacity-worker.log'
    assert metrics['limitBytes']==768*1024*1024 and metrics['oomKills']==0
    assert result['totalAddedPeakBytes']<=1_000_000_000 and ingress<=180*1024*1024
    assert backed_up and result['failures']==0
