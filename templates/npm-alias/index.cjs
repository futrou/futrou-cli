#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');

const pkg = require('./package.json');
const NAME = pkg.productName || pkg.name;

const result = spawnSync(
  'npx',
  ['--yes', 'futrou', ...process.argv.slice(2)],
  { stdio: 'inherit', shell: true }
);

process.exit(result.status ?? 1);
