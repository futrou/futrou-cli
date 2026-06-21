#!/usr/bin/env node
'use strict';

const { execFileSync } = require('child_process');
const path = require('path');
const os = require('os');

const pkg = require('./package.json');
const DIST_NAME = pkg.name;
const NAME = pkg.productName || pkg.name;

const PLATFORMS = [
  { platform: 'linux',   arch: 'x64',   goos: 'linux',   goarch: 'amd64' },
  { platform: 'linux',   arch: 'arm64',  goos: 'linux',   goarch: 'arm64' },
  { platform: 'darwin',  arch: 'x64',   goos: 'darwin',  goarch: 'amd64' },
  { platform: 'darwin',  arch: 'arm64',  goos: 'darwin',  goarch: 'arm64' },
  { platform: 'win32',   arch: 'x64',   goos: 'windows', goarch: 'amd64' },
  { platform: 'win32',   arch: 'arm64',  goos: 'windows', goarch: 'arm64' },
  { platform: 'freebsd', arch: 'x64',   goos: 'freebsd', goarch: 'amd64' },
  { platform: 'freebsd', arch: 'arm64',  goos: 'freebsd', goarch: 'arm64' },
];

function getBinaryName() {
  const platform = os.platform();
  const arch = os.arch();

  const entry = PLATFORMS.find(e => e.platform === platform && e.arch === arch);

  if (!entry) {
    const list = PLATFORMS.map(e => `  ${e.platform}/${e.arch}`).join('\n');
    process.stderr.write(
      `Unsupported platform: ${platform}/${arch}\n` +
      `${NAME} supports:\n${list}\n`
    );
    process.exit(1);
  }

  const ext = platform === 'win32' ? '.exe' : '';
  return `${DIST_NAME}-${entry.goos}-${entry.goarch}${ext}`;
}

const bin = path.join(__dirname, 'bin', getBinaryName());

try {
  execFileSync(bin, process.argv.slice(2), { stdio: 'inherit' });
} catch (err) {
  process.exit(err.status ?? 1);
}
