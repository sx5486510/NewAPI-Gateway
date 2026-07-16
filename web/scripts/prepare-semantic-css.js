const fs = require('fs');
const path = require('path');

const packageDir = path.dirname(require.resolve('semantic-ui-css/package.json'));
const sourcePath = path.join(packageDir, 'semantic.min.css');
const outputPath = path.join(packageDir, 'semantic.local.css');
const googleFontsImport =
  /@import url\(https:\/\/fonts\.googleapis\.com\/css\?family=Lato:[^)]*\);/;

const source = fs.readFileSync(sourcePath, 'utf8');
if (!googleFontsImport.test(source)) {
  throw new Error('Semantic UI Google Fonts import was not found');
}

const localCss = source.replace(googleFontsImport, '');
fs.writeFileSync(outputPath, localCss);
