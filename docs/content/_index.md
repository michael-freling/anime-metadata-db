---
title: anime-metadata-db
layout: hextra-home
---

{{< hextra/hero-badge >}}Open dataset · free hosted API{{< /hextra/hero-badge >}}

<div style="margin: 2rem 0 1.75rem;">
{{< hextra/hero-headline >}}
  Anime franchise metadata,&nbsp;<br />
  <span class="hero-gradient">structured &amp; open</span>
{{< /hextra/hero-headline >}}
</div>

<div style="margin-bottom: 3rem; max-width: 42rem;">
{{< hextra/hero-subtitle >}}
  An open dataset of anime **franchise → series → season → episode** metadata,
  with a builder CLI that compiles it and a read-only Connect RPC API that serves it.
{{< /hextra/hero-subtitle >}}
</div>

<div style="display: flex; flex-wrap: wrap; gap: 1rem; margin-bottom: 4.5rem;">
{{< hextra/hero-button text="Get started" link="docs/using-the-api" >}}
{{< hextra/hero-button text="View on GitHub →" link="https://github.com/michael-freling/anime-metadata-db" >}}
</div>

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
