# Third-Party Notices

This document records third-party research, data, and licensing notes relevant to Ant-Browser.

## Fingerprint Research References

Ant-Browser fingerprint design and compatibility research has been informed by public anti-detect browser projects and browser fingerprinting resources, including:

- Camoufox: https://github.com/daijro/camoufox
- Camoufox Python package: https://pypi.org/project/camoufox/
- BrowserForge: https://github.com/daijro/browserforge
- CloakBrowser: https://github.com/CloakHQ/CloakBrowser
- Unicode CLDR data terms: https://www.unicode.org/copyright.html

These references are used for research and compatibility analysis. Unless a file explicitly states otherwise, Ant-Browser fingerprint datasets are independently curated and maintained by the Ant-Browser project.

中文说明：`backend/internal/fingerprint/data/` 下的指纹数据为 Ant-Browser / Ant 项目自行整理与维护的数据，不是从 Camoufox、CloakBrowser、BrowserForge、CLDR、GeoIP 或其他第三方数据集直接复制而来，除非具体文件另有明确说明。

## Fingerprint Data

The files under `backend/internal/fingerprint/data/` are Ant-Browser-maintained data files. They are not direct copies of Camoufox, CloakBrowser, BrowserForge, CLDR, GeoIP, or other third-party datasets unless a file-level notice says so.

`backend/internal/fingerprint/data/client_hints.json` is a data-only UA Client Hints capability skeleton maintained from Ant-Browser / Ant-curated data. Its presence documents candidate UA-CH data only; Ant-Browser does not currently generate or apply runtime UA-CH launch arguments from this file. Runtime UA-CH spoofing requires a separate explicit implementation plus Chromium core support verification.

When a future change vendors, derives from, or mechanically transforms third-party data or code, the change must add a file-level or adjacent notice that records:

- Source URL
- Upstream project and version, commit, or release
- License
- Local modifications
- Generation process, if the file is generated

## Project-Specific Boundaries

Camoufox root project files are distributed under MPL-2.0. Camoufox Python package metadata identifies the Python package as MIT, but bundled data and fonts should still be reviewed file by file before copying.

BrowserForge is distributed under Apache-2.0. Vendoring code or data from BrowserForge requires preserving the applicable license and notice obligations.

CloakBrowser wrapper code is distributed under MIT. CloakBrowser compiled browser binaries are covered by separate proprietary binary terms and must not be redistributed, repackaged, modified, reverse engineered, or bundled with Ant-Browser.

GeoIP datasets, including GeoLite2-derived data, can have separate licensing and attribution requirements. Do not bundle a GeoIP database unless its license and update obligations have been reviewed.
