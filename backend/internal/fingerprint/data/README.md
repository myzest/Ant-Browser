# Ant-Browser Fingerprint Data

This directory contains Ant-Browser-maintained fingerprint data used by `backend/internal/fingerprint`.

The JSON files in this directory are curated for Ant-Browser and are not direct copies of Camoufox, CloakBrowser, BrowserForge, CLDR, GeoIP, or other third-party datasets unless a file-level notice says otherwise.

中文说明：本目录数据为 Ant-Browser / Ant 项目自行整理与维护的数据；除非具体文件另有明确说明，否则不视为第三方项目数据的复制、转存或机械转换。

## Files

- `client_hints.json`: data-only UA Client Hints capability skeletons by desktop platform.
- `fonts.json`: platform-oriented font name pools used for consistency checks and generated profile defaults.
- `locales.json`: country, locale, and timezone defaults used by region-aware fingerprint generation.
- `media_devices.json`: media device count templates by device class.
- `profiles.json`: weighted base profile templates.
- `screen.json`: screen resolution, color depth, and device pixel ratio distributions.
- `webgl.json`: platform-oriented WebGL vendor and renderer allowlists.

## Maintenance Rules

When updating this directory:

- Keep values consistent across platform, locale, timezone, fonts, WebGL, screen, hardware, and media device fields.
- Prefer Ant-Browser-collected or independently curated data over copying upstream datasets.
- Do not copy third-party binary fonts into this directory.
- Do not copy Camoufox, CloakBrowser, BrowserForge, CLDR, GeoIP, or other third-party data verbatim without adding a clear file-level or adjacent notice.
- If generated from a script or external source, record the source URL, upstream version or commit, license, local modifications, and generation command.

## Related Notices

See `THIRD_PARTY_NOTICES.md` at the repository root for project-wide third-party research and licensing notes.
