import type { FibelPlugin } from "@k2b/fibel";

const homepageStylesheet = "/assets/homepage.css";

export function homepagePlugin(): FibelPlugin {
  return {
    name: "rsql-homepage",
    setup(context) {
      context.headTags.push((page) =>
        page.slug === "/" ? `<link rel="stylesheet" href="${homepageStylesheet}">` : "",
      );

      const renderPage = context.services.renderPage;
      context.services.renderPage = async (page, request, currentContext) => {
        const rendered = await renderPage(page, request, currentContext);
        if (page.slug !== "/" || typeof rendered !== "string") {
          return rendered;
        }

        return rendered
          .replace('<body class="', '<body class="rsql-home-page ')
          .replace(
            '<meta property="og:type" content="article">',
            '<meta property="og:type" content="website">',
          );
      };
    },
  };
}
