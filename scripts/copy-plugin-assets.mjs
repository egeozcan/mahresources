#!/usr/bin/env node
// Copies third-party browser assets from node_modules into the plugins that
// serve them. Run by `npm run build-plugins`, which `npm run build` includes.
//
// Leaflet (pinned in package.json) goes to the location-history plugin's
// public/vendor/ directory. The plugin serves it itself so the map needs no
// script CDN, and it stays out of the main application's bundle. Leaflet
// provides panning, zoom, tiles, geographic overlays and keyboard controls,
// which nothing else in the project's dependencies or the browser offers.
// Tile URLs, attribution and caching follow OpenStreetMap's tile usage policy:
// https://operations.osmfoundation.org/policies/tiles/
//
// The e2e server loads plugins from e2e/test-plugins, and
// TestLocationHistoryE2EFixtureMatchesBundledPlugin compares the two plugin
// directories file by file, so both get the same copy.

import { cpSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const leaflet = join(root, 'node_modules', 'leaflet');

const files = [
  ['LICENSE', 'LICENSE'],
  ['dist/leaflet.js', 'leaflet.js'],
  ['dist/leaflet.js.map', 'leaflet.js.map'],
  ['dist/leaflet.css', 'leaflet.css'],
  ['dist/images', 'images'],
];

const targets = [
  'plugins/location-history/public/vendor',
  'e2e/test-plugins/location-history/public/vendor',
];

for (const target of targets) {
  const dest = join(root, target);
  rmSync(dest, { recursive: true, force: true });
  mkdirSync(dest, { recursive: true });
  for (const [from, to] of files) {
    // cpSync throws on a missing source, which fails the build.
    cpSync(join(leaflet, from), join(dest, to), { recursive: true });
  }
}
