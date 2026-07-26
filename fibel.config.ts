import { defineFibel } from "@valentinkolb/fibel";

export default defineFibel({
  title: "rsql",
  description:
    "Documentation for the rsql multi-tenant SQLite HTTP server and its Go and TypeScript clients.",
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
  headerLinks: [
    { label: "Quickstart", value: "/quickstart" },
    { label: "HTTP API", value: "/reference/http-api" },
    { label: "Hosting", value: "/hosting/multi-tenant" },
  ],
  footerLinks: [
    { label: "GitHub", value: "https://github.com/ValentinKolb/rsql" },
    {
      label: "Security",
      value: "https://github.com/ValentinKolb/rsql/security/policy",
    },
  ],
});
