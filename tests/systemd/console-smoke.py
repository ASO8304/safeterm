#!/usr/bin/env python3
"""Real shared-account Console + PTY test on the disposable installation VM."""
import errno
import os
import pty
import select
import signal
import sys
import time

if os.geteuid() != 0 or os.environ.get('SAFETERM_DISPOSABLE_HOST') != '1':
    raise SystemExit('requires disposable root CI VM')

mode = sys.argv[1]
if mode not in ('empty', 'configured'):
    raise SystemExit('expected empty or configured')
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
    command('config ?')
    command('config')
    result = command('execute disk')
    if b'result: success' not in result:
        raise RuntimeError('Gateway/Runner execution failed')
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
