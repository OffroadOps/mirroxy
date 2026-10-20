"""Local feed + unique AppId + private directory; never update the real app."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import socket
import subprocess
import threading
import uuid
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT=Path(__file__).resolve().parents[1]
parser=argparse.ArgumentParser()
parser.add_argument('--go',default='go')
parser.add_argument('--iscc',default=r'D:\DevTools\Inno Setup 7\ISCC.exe')
parser.add_argument('--version',default='0.7.3')
args=parser.parse_args()
qa=ROOT/'work'/('online-installer-test-'+uuid.uuid4().hex)
qa.mkdir(parents=True)
installed=qa/'installed'
suffix='-online-test-'+uuid.uuid4().hex[:12]
source=ROOT/'dist'/('v'+args.version)/'Mirroxy-windows-amd64.zip'
original=source.read_bytes()
state={'version':args.version,'payload':original,'bad_hash':False}

class Feed(BaseHTTPRequestHandler):
    def log_message(self,*a):pass
    def do_GET(self):
        if self.path.split('?')[0]=='/scrcpyproxy/releases.json':
            digest='0'*64 if state['bad_hash'] else hashlib.sha256(state['payload']).hexdigest()
            payload=json.dumps({'latest':state['version'],'releases':[{'version':state['version'],'files':[{'name':'Mirroxy-windows-amd64.zip','url':base+'v'+state['version']+'/Mirroxy-windows-amd64.zip','size':len(state['payload']),'sha256':digest}]}]}).encode()
        elif self.path==f"/scrcpyproxy/v{state['version']}/Mirroxy-windows-amd64.zip":payload=state['payload']
        else:self.send_error(404);return
        self.send_response(200);self.send_header('Content-Length',str(len(payload)));self.end_headers()
        for offset in range(0,len(payload),32768):self.wfile.write(payload[offset:offset+32768])

server=ThreadingHTTPServer(('127.0.0.1',0),Feed)
base=f'http://127.0.0.1:{server.server_port}/scrcpyproxy/'
threading.Thread(target=server.serve_forever,daemon=True).start()
env={**os.environ,'GOTOOLCHAIN':'local','CGO_ENABLED':'0','GOFLAGS':'-buildvcs=false'}

def execute(command,**kw):return subprocess.run(command,cwd=ROOT,env=env,creationflags=subprocess.CREATE_NO_WINDOW,**kw)
def setup(label,port=None):
    options=[str(qa/'test-online.exe'),'/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',f'/DIR={installed}',f'/LOG={qa/(label+".log")}']
    if port is not None:options.append('/PANELPORT='+str(port))
    return execute(options,timeout=180).returncode
def file_hash(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def port_read():
    result=execute([str(installed/'Mirroxy.exe'),'-setup-panel-port','read','-dir',str(installed)],capture_output=True,text=True,check=True)
    return int(result.stdout.strip())

try:
    execute([args.go,'build','-trimpath',f'-ldflags=-s -w -X main.testBuild=true -X main.feedBase={base}','-o',str(qa/'Mirroxy-bootstrap.exe'),'./cmd/bootstrap'],check=True)
    execute([args.iscc,'--quiet-progress','--define=OnlineBootstrap=1','--define=MyAppVersion=1.0.0',f'--define=AppIdSuffix={suffix}',f'--define=SourceDir={qa}',f'--define=OutputDir={qa}','--define=OutputName=test-online',str(ROOT/'packaging/Mirroxy.iss')],check=True)
    installer_hash=file_hash(qa/'test-online.exe')
    occupied=socket.socket();occupied.bind(('127.0.0.1',0));occupied.listen();port=occupied.getsockname()[1]
    if port>65000:raise RuntimeError('Test port outside installer range; rerun')
    if setup('occupied',port)==0 or (installed/'Mirroxy.exe').exists() or (installed/'data').exists():raise RuntimeError('Occupied port modified installation')
    if '无法使用面板端口' not in (qa/'occupied.log').read_text(encoding='utf-8-sig') and 'Cannot use dashboard port' not in (qa/'occupied.log').read_text(encoding='utf-8-sig'):raise RuntimeError('Occupied test failed for a reason other than the port; '+str(qa))
    occupied.close()
    print('PASS: occupied port rejects installation before any application/data writes',flush=True)
    if setup('first',port)!=0 or port_read()!=port:raise RuntimeError('First install/port persistence failed')
    print('PASS: fixed online installer fetches verified full ZIP and installs latest',flush=True)
    sentinel=installed/'data/user-data-must-stay.txt';sentinel.write_bytes(b'isolation sentinel')
    key_hash=file_hash(installed/'data/Mirroxy.key');app_hash=file_hash(installed/'Mirroxy.exe')
    db_hash=file_hash(installed/'data/Mirroxy.db')
    if setup('same-version')==0 or file_hash(installed/'Mirroxy.exe')!=app_hash or file_hash(installed/'data/Mirroxy.db')!=db_hash:raise RuntimeError('Same version replaced files/data')
    print('PASS: identical application version is rejected without changing data',flush=True)
    # Only relabel this disposable test feed, never a real release artifact.
    state['version']='0.7.4'
    with zipfile.ZipFile(io.BytesIO(original)) as archive:files={name:archive.read(name) for name in archive.namelist()}
    manifest=json.loads(files['manifest.json']);manifest['version']=state['version'];files['manifest.json']=(json.dumps(manifest,indent=2)+'\n').encode()
    files['SHA256SUMS']=''.join(hashlib.sha256(data).hexdigest()+'  '+name+'\n' for name,data in sorted(files.items()) if name!='SHA256SUMS').encode()
    output=io.BytesIO()
    with zipfile.ZipFile(output,'w',compression=zipfile.ZIP_DEFLATED) as archive:
        for name,data in files.items():archive.writestr(name,data)
    state['payload']=output.getvalue();state['bad_hash']=True
    if setup('tampered')==0 or file_hash(installed/'Mirroxy.exe')!=app_hash or file_hash(installed/'data/Mirroxy.db')!=db_hash:raise RuntimeError('Tampered package changed installation')
    print('PASS: checksum failure leaves installed application and data unchanged',flush=True)
    state['bad_hash']=False
    core=installed/'bin/runtime/Mirroxy_core_A.exe';core.write_bytes(b'test independently updated core')
    if setup('latest-changed')!=0 or port_read()!=port or sentinel.read_bytes()!=b'isolation sentinel' or file_hash(installed/'data/Mirroxy.key')!=key_hash or core.read_bytes()!=b'test independently updated core':raise RuntimeError('Upgrade failed to retain core/port/data/key')
    if file_hash(qa/'test-online.exe')!=installer_hash:raise RuntimeError('Installer unexpectedly rebuilt')
    reg_key='HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\{7D2BCE81-035C-4E1C-AAC1-58A51E667277}'+suffix+'_is1'
    result=execute(['reg','query',reg_key,'/v','DisplayVersion'],capture_output=True,text=True,check=True)
    if state['version'] not in result.stdout:raise RuntimeError('Registry did not record real catalog application version')
    print('PASS: same installer advances to changed latest version; core, data, key and port retained',flush=True)
    execute(['reg','delete',reg_key,'/f'],stdout=subprocess.DEVNULL,check=True)
    print('Test evidence retained at '+str(qa),flush=True)
finally:
    server.shutdown();server.server_close()
