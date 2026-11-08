#!/usr/bin/env python3
"""Exercise termctl's real account provisioning only on disposable CI."""
import errno
import grp
import json
import os
import pty
import pwd
import select
import signal
import subprocess
import time

if os.geteuid() != 0 or os.environ.get('SAFETERM_DISPOSABLE_HOST') != '1':
    raise SystemExit('requires disposable root CI VM')
name = 'ci-linux'
password = b'SmokeTest42'
roles = 'viewer,netops,config-viewer,config-operator'


def accounts(path):
    with open(path) as source:
        return {line.split(':', 1)[0]: line for line in source}


def identities():
    result = {}
    for kind in ('ssh', 'console'):
        with open('/etc/safeterm/access/' + kind + '/users.json', 'rb') as source:
            result[kind] = source.read()
    return result


before = {path: accounts(path) for path in ('/etc/passwd', '/etc/shadow')}
before_identities = identities()
if name in before['/etc/passwd']:
    raise SystemExit('disposable Console test account already exists')

# Existing users must be rejected before terminal input or host changes.
for existing in ('root', 'safeterm', 'nobody'):
    result = subprocess.run(['/usr/local/bin/termctl', 'user', 'add', 'console',
                             '--os-user', existing, '--roles', 'viewer'],
                            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=10)
    if result.returncode == 0:
        raise RuntimeError('existing account was adopted')
    for path, entries in before.items():
        if accounts(path) != entries:
            raise RuntimeError('existing account rejection changed OS credentials')

pid, fd = pty.fork()
if pid == 0:
    os.execv('/usr/local/bin/termctl', ['termctl', 'user', 'add', 'console',
                                      '--os-user', name, '--id', 'ci-admin', '--roles', roles])
transcript = b''
pending = b''


def expect(marker):
    global pending, transcript
    deadline = time.monotonic() + 60
    while marker not in pending:
        if time.monotonic() >= deadline:
            raise RuntimeError('Console provisioning timed out waiting for ' + repr(marker))
        if not select.select([fd], [], [], 0.2)[0]:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError as error:
            if error.errno == errno.EIO:
                raise RuntimeError('Console provisioning exited early') from error
            raise
        if not data:
            raise RuntimeError('Console provisioning EOF')
        transcript += data
        pending += data
    end = pending.index(marker) + len(marker)
    result, pending = pending[:end], pending[end:]
    return result


try:
    expect(b'New password: ')
    os.write(fd, password + b'\r')
    expect(b'Confirm password: ')
    os.write(fd, password + b'\r')
    expect(b'Local Console user created:')
    _, status = os.waitpid(pid, 0)
    pid = 0
    if not os.WIFEXITED(status) or os.WEXITSTATUS(status) != 0:
        raise RuntimeError('Console provisioning failed')
    if password in transcript:
        raise RuntimeError('Console provisioning echoed the password')
    account = pwd.getpwnam(name)
    if account.pw_uid == 0 or account.pw_shell != '/usr/local/bin/safeterm-console':
        raise RuntimeError('Console account has unsafe UID or login shell')
    group_ids = os.getgrouplist(name, account.pw_gid)
    groups = {grp.getgrgid(gid).gr_name for gid in group_ids}
    if groups != {name, 'safeterm-clients', 'safeterm-gateway'}:
        raise RuntimeError('Console account received unintended groups')
    for path, entries in before.items():
        after = accounts(path)
        if {key: value for key, value in after.items() if key != name} != entries:
            raise RuntimeError('provisioning changed existing Linux accounts')
    if identities() != before_identities:
        raise RuntimeError('provisioning changed SSH or legacy Console credentials')
    shadow = accounts('/etc/shadow')[name].split(':')[1]
    if not shadow or shadow.startswith(('!', '*')):
        raise RuntimeError('new Console account did not get its Linux password')
    with open('/etc/safeterm/access/console/linux-users.json') as source:
        mapping = next(user for user in json.load(source)['users'] if user['os_user'] == name)
    if mapping['user_id'] != 'ci-admin' or mapping['roles'] != roles.split(','):
        raise RuntimeError('Console roles or identity were not provisioned')
    # Retrying a completed add must leave even its new Linux credentials intact.
    snapshots = {path: accounts(path) for path in before}
    result = subprocess.run(['/usr/local/bin/termctl', 'user', 'add', 'console',
                             '--os-user', name, '--id', 'another-id', '--roles', 'viewer'],
                            stdin=subprocess.DEVNULL, capture_output=True, timeout=10)
    if result.returncode == 0 or any(accounts(path) != value for path, value in snapshots.items()):
        raise RuntimeError('existing Console account was changed on retry')
    print('New-only Console account provisioning: PASS')
except BaseException:
    # Bounded and redacted diagnostics; never print shadow contents or secrets.
    import sys
    sys.stderr.write(transcript[-16000:].replace(password, b'[REDACTED]').decode('utf-8', 'replace'))
    raise
finally:
    os.close(fd)
    if pid:
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        os.waitpid(pid, 0)
