# Leaflet 1.9.4

Vendored distribution from https://unpkg.com/leaflet@1.9.4/dist/ with the
upstream BSD 2-Clause license from the v1.9.4 tag:
https://github.com/Leaflet/Leaflet/tree/v1.9.4

The files are unmodified. Leaflet provides panning, zoom, tiles, geographic
overlays and keyboard controls, which have no equivalent in the project's
existing dependencies or browser APIs. Keeping it inside the plugin avoids a
runtime script CDN and changes to the main application's bundle.

When updating, download the pinned distribution, retain LICENSE and copy the
whole plugin into `e2e/test-plugins/location-history`. The fixture parity test
checks every file. Follow OpenStreetMap's tile usage policy for tile URLs,
attribution and caching: https://operations.osmfoundation.org/policies/tiles/
