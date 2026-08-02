![Futrou CLI Banner](.github/assets/banner.jpg)

# Futrou CLI

The **Futrou CLI** allows you to manage your Futrou projects and infrastructure from your terminal.

## Installation
To install the CLI globally, run:

```bash
npm install -g futrou
```

Or use any other favorite package manager:

```bash
yarn global add futrou
```

## Usage

Once installed, you can access the CLI using the `npx futrou` command.

## Infrastructure as code

The npm package also exports a small JavaScript API for building `futrou.json`:

```js
const { Config } = require('futrou');

const config = new Config({ workspace: 'acme', project: 'website' })
  .serverlet({ name: 'web', image: 'nginx:latest', ram: 128, cpu: 100 });

require('fs').writeFileSync('futrou.json', config.toJson() + '\n');
```

`toJs()` returns an ESM default-export module and `toJsonSchema()` returns the
generated Futrou JSON Schema bundled with the package.

## Documentation

For more detailed guides on how to use the CLI, please visit [futrou.com](https://futrou.com/docs).

## License
[MIT License](LICENSE)
