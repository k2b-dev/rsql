import { defaultPlugins, defineFibel } from "@valentinkolb/fibel";
import { imprintPlugin } from "@valentinkolb/fibel/plugins";
import { homepagePlugin } from "./plugins/homepage";

const siteUrl = process.env.RSQL_DOCS_SITE_URL?.replace(/\/+$/, "");

export default defineFibel({
  title: "rsql",
  description:
    "Run isolated SQLite databases behind one HTTP service, with Go and TypeScript clients.",
  siteUrl,
  content: "docs",
  assets: "assets",
  locales: [{ code: "en", label: "English" }],
  defaultLocale: "en",
  routing: {
    basePath: "",
    internalPath: "/_fibel",
    assetsPath: "/assets",
  },
  theme: {
    defaultMode: "light",
    cookieName: "rsql_docs_theme",
  },
  seo: {
    ogImage: "/assets/rsql-social.png",
  },
  headerLinks: [
    { label: "Quickstart", value: "/quickstart" },
    { label: "HTTP API", value: "/reference/http-api" },
    { label: "Hosting", value: "/hosting/multi-tenant" },
  ],
  footerLinks: [
    { label: "GitHub", value: "https://github.com/k2b-dev/rsql" },
    {
      label: "Security",
      value: "https://github.com/k2b-dev/rsql/security/policy",
    },
  ],
  plugins: [
    ...defaultPlugins(),
    imprintPlugin({ url: "https://impressum.valentin-kolb.com" }),
    homepagePlugin(),
  ],
});
