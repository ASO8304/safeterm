#!/usr/bin/env python3
"""Real shared-account Console + PTY test on the disposable installation VM."""
import errno
import os
import pty
import select
import signal
import socket
import shutil
import subprocess
import sys
import time

if os.geteuid() != 0 or os.environ.get('SAFETERM_DISPOSABLE_HOST') != '1':
    raise SystemExit('requires disposable root CI VM')

mode = sys.argv[1]
if mode not in ('empty', 'configured', 'site', 'logical', 'logs', 'services'):
    raise SystemExit('expected empty, configured, site, logical, logs or services')
pid, fd = pty.fork()
if pid == 0:
    os.execv('/usr/bin/sudo', ['sudo', '-iu', 'safeterm'])
pending = b''
transcript = b''


def expect(marker, timeout=20):
    global pending, transcript
    deadline = time.monotonic() + timeout
    while marker not in pending:
        if time.monotonic() >= deadline:
            raise RuntimeError('timed out waiting for ' + repr(marker))
        readable, _, _ = select.select([fd], [], [], 0.2)
        if not readable:
            continue
        try:
            data = os.read(fd, 65536)
        except OSError as error:
            if error.errno == errno.EIO:
                raise RuntimeError('Console exited before ' + repr(marker)) from error
            raise
        if not data:
            raise RuntimeError('Console EOF before ' + repr(marker))
        pending += data
        transcript += data
    end = pending.index(marker) + len(marker)
    result, pending = pending[:end], pending[end:]
    return result


def command(text, prompt=b'\nsafeterm::ci-admin> '):
    os.write(fd, text.encode('ascii') + b'\r')
    result = expect(prompt)
    if b'error:' in result or b'Permission denied' in result:
        raise RuntimeError('command failed: ' + text)
    return result


try:
    expect(b'Username: ')
    os.write(fd, b'ci-admin\r')
    expect(b'Password: ')
    os.write(fd, b'SmokeTest42\r')
    expect(b'\nsafeterm::ci-admin> ')
    if mode == 'empty':
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen(1)
            listener.settimeout(5)
            result = command('execute telnet 127.0.0.1 ' + str(listener.getsockname()[1]))
            if b'TCP port is open' not in result or b'result: success' not in result:
                raise RuntimeError('native TCP test failed')
            conn, _ = listener.accept()
            with conn:
                conn.settimeout(2)
                if conn.recv(1) != b'':
                    raise RuntimeError('TCP probe sent application data')
        # Delay only this disposable loopback destination so Ctrl+C crosses the
        # real Console -> Gateway -> Runner -> native dial cancellation path.
        firewall = shutil.which('iptables')
        if firewall is None:
            raise RuntimeError('disposable TCP cancellation test requires iptables')
        with socket.socket() as blocked:
            blocked.bind(('127.0.0.1', 0))
            blocked.listen(1)
            port = str(blocked.getsockname()[1])
            rule = ['OUTPUT', '-p', 'tcp', '-d', '127.0.0.1', '--dport', port,
                    '--syn', '-m', 'comment', '--comment', 'safeterm-ci-cancel', '-j', 'DROP']
            subprocess.run([firewall, '-w', '5', '-I', *rule], check=True)
            try:
                os.write(fd, ('execute telnet 127.0.0.1 ' + port + '\r').encode('ascii'))
                expect(('Trying 127.0.0.1:' + port).encode('ascii'))
                os.write(fd, b'\x03')
                expect(b'\nsafeterm::ci-admin> ')
                if b'user: ci-admin' not in command('whoami'):
                    raise RuntimeError('Ctrl+C closed SafeTerm instead of only the probe')
            finally:
                subprocess.run([firewall, '-w', '5', '-D', *rule], check=True)
        result = command('execute ping 127.0.0.1 -c 1')
        if b'result: success' not in result:
            raise RuntimeError('positional ping failed')
        result = command('execute ?')
        if b'Authorized service commands' in result or b'execute service ' in result:
            raise RuntimeError('package registered implicit service commands')
    command('config ?')
    command('config')
    result = command('execute system_info disk')
    if b'result: success' not in result:
        raise RuntimeError('Gateway/Runner execution failed')
    if mode == 'logs':
        os.write(fd, b'execute logs fresh-log main\r')
        result = expect(b'CI log fixture')
        if b'error:' in result:
            raise RuntimeError('registered log stream failed')
        os.write(fd, b'\x03')
        expect(b'\nsafeterm::ci-admin> ')
    if mode == 'services':
        result = command('execute service restart ?')
        if b'application-stack' not in result:
            raise RuntimeError('service group missing from contextual help')
        for action in ('start', 'status', 'restart', 'stop'):
            result = command('execute service ' + action + ' application-st\t')
            if b'result: success exit=0' not in result or b'application-stack' not in result:
                raise RuntimeError('service group action failed: ' + action)
            if b'ci-chain-' in result or b'.service' in result:
                raise RuntimeError('physical service name leaked to Console')
    if mode == 'site':
        result = command('execute site_uname')
        if b'Linux' not in result or b'result: success exit=0' not in result:
            raise RuntimeError('isolated custom Runner execution failed')
        result = command('execute site_status')
        if b'result: success exit=0' not in result:
            raise RuntimeError('profile-scoped typed systemd operation failed')
    if mode == 'configured':
        result = command('config ?')
        if b'fresh-options' not in result:
            raise RuntimeError('registered resource missing from config help')
        # Successful context entry proves that Tab expanded the registered ID.
        command('config fresh\t', b'\nsafeterm::ci-admin(fresh-options)> ')
        command('edit Ser\t', b'\nsafeterm::ci-admin(fresh-options/Server)> ')
        result = command('show', b'\nsafeterm::ci-admin(fresh-options/Server)> ')
        if b'8080' not in result:
            raise RuntimeError('registered INI value not read')
        command('set Po\t9090', b'\nsafeterm::ci-admin(fresh-options/Server)> ')
        result = command('diff', b'\nsafeterm::ci-admin(fresh-options/Server)> ')
        if b'8080 -> 9090' not in result:
            raise RuntimeError('completed set command did not stage expected edit')
        result = command('end')
        if b'Configuration saved.' not in result:
            raise RuntimeError('INI change did not commit')
    if mode == 'logical':
        result = command('config ?')
        if b'tunnel' not in result or b'network-settings' in result:
            raise RuntimeError('logical config paths not exposed correctly')
        command('config tu\t', b'\nsafeterm::ci-admin(tunnel)> ')
        result = command('show', b'\nsafeterm::ci-admin(tunnel)> ')
        if b'Value_1' not in result or b'TCPMSS' in result:
            raise RuntimeError('physical field name exposed instead of alias')
        command('set Value_1\t1400', b'\nsafeterm::ci-admin(tunnel)> ')
        command('set Value_2 443', b'\nsafeterm::ci-admin(tunnel)> ')
        result = command('diff', b'\nsafeterm::ci-admin(tunnel)> ')
        if b'Value_1: 1360 -> 1400' not in result:
            raise RuntimeError('alias diff missing')
        command('end')
        command('config application c\t', b'\nsafeterm::ci-admin(application/cat)> ')
        command('set enabled false', b'\nsafeterm::ci-admin(application/cat)> ')
        command('end')
    os.write(fd, b'exit\r')
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        done, status = os.waitpid(pid, os.WNOHANG)
        if done:
            if not os.WIFEXITED(status) or os.WEXITSTATUS(status) != 0:
                raise RuntimeError('Console exited unsuccessfully')
            pid = 0
            break
        time.sleep(0.1)
    if pid:
        raise RuntimeError('Console did not exit')
    print('Console PTY ' + mode + ': PASS')
except Exception:
    # Password entry has no echo. Never print the password we sent.
    sys.stderr.write(transcript.replace(b'SmokeTest42', b'[REDACTED]').decode('utf-8', 'replace'))
    raise
finally:
    os.close(fd)
    if pid:
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        os.waitpid(pid, 0)
