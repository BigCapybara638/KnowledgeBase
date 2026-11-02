from harness import Harness
import json
import re

def test_connect_and_recreate():
    with Harness() as h:
        c = h.client
        result = c.request('/admin/categories', 'POST', form={'title': 'Интеграция', 'order': '0'})
        assert result.status == 200
        category = re.search(r'/admin/categories/([0-9a-f-]{36})"', result.body)[1]
        article = c.create_article(category, 'Проверка двух Compose-проектов', 'Сессия и статья переживают пересоздание контейнера')
        assert c.request(article).status == 200
        second = h.compose(h.kb_project, 'run', '--rm', '--no-deps', '-T', 'knowledgebase-app', 'serve', check=False)
        assert second.returncode != 0, 'second server opened the same volume'
        assert b'resource in use' in second.stderr, 'second server failed for a reason unrelated to the instance lock'
        h.recreate_app_with_new_ip()
        response=c.request(article)
        assert response.status == 200 and 'Проверка двух Compose-проектов' in response.body, 'session or article lost after restart'
        h.docker('kill', '--signal=KILL', h.container(h.kb_project,'knowledgebase-app'))
        h.compose(h.kb_project,'up','-d')
        h.wait('/readyz','ready')
        response=c.request(article)
        assert response.status == 200 and 'Проверка двух Compose-проектов' in response.body, 'committed data or session lost after SIGKILL'
        h.compose(h.kb_project, 'exec', '-T', 'knowledgebase-app', '/app/kb', 'backup')
        metrics = json.loads(h.compose(h.kb_project, 'exec', '-T', 'knowledgebase-app', '/app/kb', 'metrics').stdout)
        assert metrics['limitBytes'] == 768*1024*1024
        assert metrics['oomKills'] == 0
        print('PASS: HTTPS, two Compose projects, private login, article, instance lock, changed IP, unchanged client/proxy, persistent session, backup and cgroup limit')

if __name__ == '__main__':
    test_connect_and_recreate()
