import type { FibelPlugin } from "@valentinkolb/fibel";

const homepageStylesheet = "/assets/homepage.css";

export function homepagePlugin(): FibelPlugin {
  return {
    name: "rsql-homepage",
    setup(context) {
      context.headTags.push((page) =>
        page.slug === "/" ? `<link rel="stylesheet" href="${homepageStylesheet}">` : "",
      );

      const renderPage = context.services.renderPage;
      context.services.renderPage = (page, request, currentContext) => {
        const html = renderPage(page, request, currentContext);
        if (page.slug !== "/") {
          return html;
        }

        return html
          .replace('<body class="', '<body class="rsql-home-page ')
          .replace(
            '<meta property="og:type" content="article">',
            '<meta property="og:type" content="website">',
          );
      };
    },
  };
}
