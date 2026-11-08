#!/usr/bin/env python3
"""Password boundary integration; only on the disposable installation VM."""
import errno
import json
import os
import pty
import re
import select
import signal
import subprocess
import sys
import tempfile
import time

if os.geteuid() != 0 or os.environ.get('SAFETERM_DISPOSABLE_HOST') != '1':
    raise SystemExit('requires disposable root CI VM')
mode = sys.argv[1]
if mode not in ('ssh', 'console', 'linux'):
    raise SystemExit('expected ssh, console or linux')
old, new = b'SmokeTest42', b'SmokeChanged43'
prompt = b'\nsafeterm::ci-admin> '


def credentials():
    result = {}
    for kind in ('ssh', 'console'):
        with open('/etc/safeterm/access/' + kind + '/users.json') as source:
            result[kind] = json.load(source)
    with open('/etc/shadow') as source:
        result['linux'] = next((line for line in source if line.startswith('ci-linux:')), '')
    return result


class Session:
    def __init__(self, argv):
        self.pid, self.fd = pty.fork()
        self.data = b''
        self.transcript = b''
        if not self.pid:
            os.environ['TERM'] = 'dumb'
            os.execv(argv[0], argv)

    def send(self, value):
        os.write(self.fd, value + b'\r')

    def expect(self, marker):
        deadline = time.monotonic() + 25
        while marker not in self.data:
            if time.monotonic() >= deadline:
                raise RuntimeError('password test timed out waiting for ' + repr(marker))
            if not select.select([self.fd], [], [], 0.2)[0]:
                continue
            try:
                data = os.read(self.fd, 65536)
            except OSError as error:
                if error.errno == errno.EIO:
                    raise RuntimeError('session exited during password test') from error
                raise
            if not data:
                raise RuntimeError('unexpected EOF during password test')
            self.transcript += data
            self.data += re.sub(rb'\x1b\[[0-9;]*m', b'', data)
        end = self.data.index(marker) + len(marker)
        result, self.data = self.data[:end], self.data[end:]
        return result

    def change(self, current, replacement):
        self.send(b'passwd')
        self.expect(b'Current password: ')
        self.send(current)
        self.expect(b'New password: ')
        self.send(replacement)
        self.expect(b'Confirm new password: ')
        self.send(replacement)
        return self.expect(prompt)

    def close(self):
        try:
            os.kill(self.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        os.waitpid(self.pid, 0)
        os.close(self.fd)


with tempfile.NamedTemporaryFile() as known:
    scan = subprocess.run(['/usr/bin/ssh-keyscan', '-p', '2222', '127.0.0.1'],
                          check=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    known.write(scan.stdout)
    known.flush()

    def login(password=old, rejected=False):
        if mode == 'ssh':
            argv = ['/usr/bin/ssh', '-tt', '-p', '2222', '-o', 'PreferredAuthentications=password',
                    '-o', 'PubkeyAuthentication=no', '-o', 'NumberOfPasswordPrompts=1',
                    '-o', 'StrictHostKeyChecking=yes', '-o', 'UserKnownHostsFile=' + known.name,
                    'ci-admin@127.0.0.1']
        else:
            argv = ['/usr/bin/sudo', '-iu', 'ci-linux' if mode == 'linux' else 'safeterm']
        session = Session(argv)
        try:
            if mode == 'ssh':
                session.expect(b'password: ')
                session.send(password)
                if rejected:
                    session.expect(b'Permission denied')
                    return session
            elif mode == 'console':
                session.expect(b'Username: ')
                session.send(b'ci-admin')
                session.expect(b'Password: ')
                session.send(password)
            session.expect(prompt)
            return session
        except BaseException:
            session.close()
            raise

    session = login()
    try:
        before = credentials()
        denied = session.change(b'WrongPassword42', new)
        if b'Error:' not in denied or credentials() != before:
            raise RuntimeError('wrong current password changed credentials')
        changed = session.change(old, new)
        if b'Password changed successfully.' not in changed:
            raise RuntimeError('self-service password change failed: ' + repr(changed))
        after = credentials()
        for kind in before:
            if (before[kind] != after[kind]) != (kind == mode):
                raise RuntimeError('password change touched wrong credential store: ' + kind)
        session.send(b'execute system_info host')
        result = session.expect(prompt)
        if b'Linux' not in result or b'Error:' in result:
            raise RuntimeError('password change broke authenticated session')
        if mode != 'linux':
            if b'Password changed successfully.' not in session.change(new, old):
                raise RuntimeError('could not restore disposable SafeTerm password')
        for secret in (old, new, b'WrongPassword42'):
            if secret in session.transcript:
                raise RuntimeError('password was echoed to the terminal')
    finally:
        session.close()
    if mode == 'ssh':
        # The broker stays running: password authentication must reload changes.
        session = login()
        try:
            if b'Password changed successfully.' not in session.change(old, new):
                raise RuntimeError('SSH password update failed')
        finally:
            session.close()
        denied = login(old, rejected=True)
        denied.close()
        accepted = login(new)
        try:
            if b'Password changed successfully.' not in accepted.change(new, old):
                raise RuntimeError('fresh SSH password was not accepted')
        finally:
            accepted.close()
print('Self-service password ' + mode + ': PASS')
