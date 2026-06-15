// Docusaurus plugin: injects Tailwind CSS into the PostCSS pipeline.
// Uses Tailwind v3 with preflight disabled so Infima base styles are untouched.
module.exports = function () {
  return {
    name: 'tailwind-plugin',
    configurePostCss(postcssOptions) {
      postcssOptions.plugins.push(
        require('tailwindcss'),
        require('autoprefixer'),
      );
      return postcssOptions;
    },
  };
};
