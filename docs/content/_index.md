---
title: anime-metadata-db
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <div class="hx-w-2 hx-h-2 hx-rounded-full hx-bg-primary-400"></div>
  Open dataset · free hosted API
{{< /hextra/hero-badge >}}

<div class="hx-mt-6 hx-mb-6">
{{< hextra/hero-headline >}}
  Anime franchise metadata,&nbsp;<br class="sm:hx-block hx-hidden" />
  <span class="hero-gradient">structured &amp; open</span>
{{< /hextra/hero-headline >}}
</div>

<div class="hx-mb-12">
{{< hextra/hero-subtitle >}}
  An open dataset of anime **franchise → series → season → episode** metadata,&nbsp;<br class="sm:hx-block hx-hidden" />
  with a builder CLI that compiles it and a read-only Connect RPC API that serves it.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx-mb-6 hx-flex hx-flex-wrap hx-gap-4">
{{< hextra/hero-button text="Get started" link="docs/using-the-api" >}}
{{< hextra/hero-button text="View on GitHub →" link="https://github.com/michael-freling/anime-metadata-db" >}}
</div>

<div class="hx-mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Using the API"
    icon="code"
    subtitle="Query the hosted, read-only service with a plain HTTP POST + JSON — no client library or codegen."
    link="docs/using-the-api"
  >}}
  {{< hextra/feature-card
    title="Using the dataset"
    icon="database"
    subtitle="Read the committed YAML directly and learn the Franchise → Series → Season → Episode model."
    link="docs/using-the-dataset"
  >}}
  {{< hextra/feature-card
    title="Building the dataset"
    icon="terminal"
    subtitle="Compile the dataset and author your own entries with the deterministic builder CLI."
    link="docs/building-the-dataset"
  >}}
{{< /hextra/feature-grid >}}

<div class="hx-mt-16 hx-mb-4 hx-text-center">
  <span class="hx-text-sm hx-font-semibold hx-tracking-wide hx-text-gray-400 dark:hx-text-gray-500">TRY IT — ONE CURL, NO SETUP</span>
</div>

```sh
curl -X POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/GetHealth \
  -H 'Content-Type: application/json' -d '{}'
```

```json
{"status":"ok","version":"<commit>","stats":{"franchises":1,"series":3,"seasons":9,"episodes":124}}
```
