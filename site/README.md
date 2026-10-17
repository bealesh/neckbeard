# neckbeard.sh

The website. Plain HTML and one stylesheet, served as static assets from a
Cloudflare Worker. No build step, no dependencies, no JavaScript.

```
site/
  wrangler.jsonc     Worker config: assets directory, custom domains
  public/            what gets served
    index.html
    404.html
    style.css
    _headers         security headers + font caching
    _redirects       www → apex
    fonts/           IBM Plex Mono 400/500, latin, woff2 (SIL OFL, see LICENSE-OFL.txt)
    neckbeard.png    the logo, also the og:image
    favicon.png, apple-touch-icon.png
```

## Local

```sh
cd site && npx wrangler dev
```

or just open `public/index.html`.

## Deploy

`.github/workflows/site.yml` runs `wrangler deploy` on every push to `main`
that touches `site/`. It needs two repository secrets, kept in the
`neckbeard.sh` environment so they're only ever exposed to that job:

| secret | value |
|---|---|
| `CLOUDFLARE_ACCOUNT_ID` | Workers & Pages → Overview, right sidebar |
| `CLOUDFLARE_API_TOKEN` | a token with *Workers Scripts: Edit*, *Workers Routes: Edit*, and *Zone: Read* on the neckbeard.sh zone |

First deploy from a laptop works too: `cd site && npx wrangler login && npx wrangler deploy`.
The `custom_domain` routes in `wrangler.jsonc` create the DNS records and
certificates for `neckbeard.sh` and `www.neckbeard.sh` on first deploy; the
zone just has to be on the account already.

## Design

Three colors (ground `#070807`, text `#c9d1c9` / quiet `#7a847a`, phosphor
`#4af626`), one typeface, and four things the page is allowed to do: the `$`
prompt, a blinking block cursor, 1px-ruled tables with no fills, and status as
plain words. Every number on the page comes from the README or a real run;
nothing is illustrative.
