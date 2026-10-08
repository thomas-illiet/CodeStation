/** Safe Markdown rendering for package README content received from the registry. */
import MarkdownIt from "markdown-it";

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true
});

/** Renders Markdown while keeping embedded HTML disabled. */
export function renderPackageMarkdown(source: string): string {
  return markdown.render(source);
}
