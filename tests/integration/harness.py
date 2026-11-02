"""Disposable local Docker/TLS fixtures. Never uses production project names or data."""
from dataclasses import dataclass
from html import unescape
from pathlib import Path
import http.cookiejar
import json
import os
import re
import secrets
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
DOCKER = ['docker', '--host=unix:///var/run/docker.sock']
NGINX = 'nginx:1.28-alpine@sha256:a8b39bd9cf0f83869a2162827a0caf6137ddf759d50a171451b335cecc87d236'

@dataclass
class Response:
    status: int
    body: str
    headers: object

class Client:
    def __init__(self, origin, certificate):
        self.origin = origin
        self.csrf = ''
        self.cookies = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=certificate)), urllib.request.HTTPCookieProcessor(self.cookies))

    def request(self, path, method='GET', form=None, payload=None):
        headers = {'Origin': self.origin, 'Referer': self.origin + '/', 'X-CSRF-Token': self.csrf}
        data = None
        if form is not None:
            data = urllib.parse.urlencode({**form, 'csrf_token': self.csrf}).encode()
            headers['Content-Type'] = 'application/x-www-form-urlencoded'
        if payload is not None:
            data = json.dumps(payload, ensure_ascii=False).encode()
            headers.update({'Content-Type': 'application/json', 'Accept': 'application/json'})
        req = urllib.request.Request(self.origin + path, data=data, method=method, headers=headers)
        try:
            response = self.opener.open(req, timeout=25)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read().decode('utf-8')
            token = re.search(r'<meta name="csrf-token" content="([^"]+)"', body)
            if token:
                self.csrf = unescape(token[1])
            return Response(response.status, body, response.headers)

    def login(self, username, password):
        assert self.request('/login').status == 200
        result = self.request('/login', 'POST', form={'login': username, 'password': password})
        assert result.status == 200, ('login', result.status)

    def create_article(self, category, title, text):
        page = self.request('/drafts', 'POST', form={})
        assert page.status == 200, ('create draft', page.status)
        state = json.loads(re.search(r'id="editor-state">(.*?)</script>', page.body, re.S)[1])
        doc = {'type': 'doc', 'content': [{'type': 'paragraph', 'content': [{'type': 'text', 'text': text}]}]}
        result = self.request('/drafts/' + state['id'], 'PUT', payload={'title': title, 'categoryId': category, 'expectedVersion': state['version'], 'document': doc})
        assert result.status == 200, ('save', result.status)
        version = json.loads(result.body)['version']
        result = self.request('/drafts/' + state['id'] + '/publish', 'POST', payload={'expectedDraftVersion': version, 'expectedArticleVersion': state['baseVersion']})
        assert result.status == 200, ('publish', result.status)
        return json.loads(result.body)['url']

class Harness:
    def __init__(self):
        self.token = uuid.uuid4().hex[:12]
        self.kb_project = 'kbtest-' + self.token
        self.proxy_project = 'kbproxy-' + self.token
        self.network = 'kb-ingress-' + self.token
        self.reserve = 'kb-reserve-' + self.token
        self.tmp = tempfile.TemporaryDirectory(prefix='kb-integration-')
        self.cert_dir = Path(self.tmp.name)
        self.env = {k: v for k, v in os.environ.items() if k not in ('DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_TLS', 'DOCKER_TLS_VERIFY', 'DOCKER_CERT_PATH')}
        self.env['KB_PUBLIC_URL'] = 'https://localhost'
        self.env.update(KB_IMAGE=os.environ.get('KB_IMAGE', 'knowledgebase:test'), KB_NETWORK_NAME=self.network, KB_ROUTE_FILE=str(ROOT/'deploy/nginx-site.conf.example'), KB_UPSTREAM_FILE=str(ROOT/'deploy/nginx-upstream.conf.example'), KB_TLS_DIR=str(self.cert_dir))
        self.kb_files = ['-f', str(ROOT/'deploy/compose.yaml'), '-f', str(ROOT/'tests/integration/compose.kb.yaml')]
        self.proxy_files = ['-f', str(ROOT/'tests/integration/compose.proxy.yaml')]
        self.password = secrets.token_urlsafe(24)
        self.origin = ''
        self.client = None

    def docker(self, *args, input=None, check=True):
        result = subprocess.run([*DOCKER, *args], env=self.env, input=input, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if check and result.returncode:
            raise RuntimeError('Docker operation failed: ' + ' '.join(args[:3]) + '\n' + result.stderr.decode()[-1600:])
        return result

    def compose(self, project, *args, input=None, check=True):
        files = self.kb_files if project == self.kb_project else self.proxy_files
        return self.docker('compose', '-p', project, *files, *args, input=input, check=check)

    def __enter__(self):
        try:
            assert (ROOT/'deploy/compose.yaml').is_file(), 'production Compose is not implemented'
            assert (ROOT/'deploy/nginx-site.conf.example').is_file(), 'Nginx integration fragment is not implemented'
            subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1', '-subj', '/CN=localhost', '-addext', 'subjectAltName=IP:127.0.0.1,DNS:localhost', '-keyout', str(self.cert_dir/'key.pem'), '-out', str(self.cert_dir/'cert.pem')], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.docker('network', 'create', self.network)
            subnet = json.loads(self.docker('network', 'inspect', self.network).stdout)[0]['IPAM']['Config'][0]['Subnet']
            self.docker('network', 'rm', self.network)
            self.docker('network', 'create', '--subnet', subnet, self.network)
            self.compose(self.proxy_project, 'up', '-d')
            address = self.compose(self.proxy_project, 'port', 'nginx', '443').stdout.decode().strip()
            self.origin = 'https://' + address
            self.env['KB_PUBLIC_URL'] = self.origin
            self.client = Client(self.origin, str(self.cert_dir/'cert.pem'))
            self.wait('/client/health', 'client-ok')
            self.proxy_id = self.container(self.proxy_project, 'nginx')
            self.client_id = self.container(self.proxy_project, 'client-software')
            self.compose(self.kb_project, 'run', '--rm', '--no-deps', '-T', 'knowledgebase-app', 'admin', 'create', '--login', 'admin', '--name', 'Test admin', '--password-stdin', input=(self.password+'\n').encode())
            self.compose(self.kb_project, 'up', '-d')
            self.wait('/readyz', 'ready')
            self.client.login('admin', self.password)
            fresh = secrets.token_urlsafe(24)
            result = self.client.request('/password', 'POST', form={'old_password': self.password, 'new_password': fresh})
            assert result.status == 200, ('password change', result.status)
            self.password = fresh
            return self
        except BaseException:
            self.cleanup()
            raise

    def container(self, project, service):
        return self.compose(project, 'ps', '-a', '-q', service).stdout.decode().strip()

    def wait(self, path, expected='', timeout=40):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            try:
                response = self.client.request(path)
                if response.status == 200 and expected in response.body:
                    return response
            except (urllib.error.URLError, ConnectionError, TimeoutError):
                pass
            time.sleep(.3)
        raise AssertionError('HTTP route did not recover: ' + path)

    def recreate_app_with_new_ip(self):
        old_id = self.container(self.kb_project, 'knowledgebase-app')
        info = json.loads(self.docker('inspect', old_id).stdout)[0]
        old_ip = info['NetworkSettings']['Networks'][self.network]['IPAddress']
        self.compose(self.kb_project, 'rm', '-s', '-f', 'knowledgebase-app')
        self.docker('run', '-d', '--name', self.reserve, '--network', self.network, '--ip', old_ip, '--entrypoint', '/bin/sleep', NGINX, '90')
        self.compose(self.kb_project, 'up', '-d')
        new_id = self.container(self.kb_project, 'knowledgebase-app')
        new_ip = json.loads(self.docker('inspect', new_id).stdout)[0]['NetworkSettings']['Networks'][self.network]['IPAddress']
        assert new_ip != old_ip, 'test did not exercise an IP change'
        self.wait('/readyz', 'ready')
        assert self.container(self.proxy_project, 'nginx') == self.proxy_id, 'proxy restarted'
        assert self.container(self.proxy_project, 'client-software') == self.client_id, 'client software restarted'
        self.wait('/client/health', 'client-ok')

    def cleanup(self):
        self.docker('rm', '-f', self.reserve, check=False)
        self.compose(self.kb_project, 'down', '-v', '--remove-orphans', check=False)
        self.compose(self.proxy_project, 'down', '-v', '--remove-orphans', check=False)
        self.docker('network', 'rm', self.network, check=False)
        self.tmp.cleanup()

    def __exit__(self, *_):
        self.cleanup()
