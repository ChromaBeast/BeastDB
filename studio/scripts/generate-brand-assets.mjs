import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import sharp from "sharp";

const brandDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../public/brand");

function svg(title, viewBox, content) {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}" role="img" aria-label="${title}">\n${content}\n</svg>\n`;
}

async function markBody(theme) {
  const source = await readFile(path.join(brandDir, `beastdb-mark-${theme}.svg`), "utf8");
  return source.replace(/^<svg[^>]*>/, "").replace(/<\/svg>\s*$/, "").replace(/<title[^>]*>[\s\S]*?<\/title>/, "").replace(/<desc[^>]*>[\s\S]*?<\/desc>/, "");
}

function wordmark(x, y, fontSize, beastColor, dbColor) {
  const common = `font-family="Arial, Helvetica, sans-serif" font-size="${fontSize}" font-weight="800" letter-spacing="-5"`;
  return `<text x="${x}" y="${y}" ${common} fill="${beastColor}">Beast</text>
  <text x="${x + fontSize * 2.6}" y="${y}" ${common} fill="${dbColor}">DB</text>`;
}

async function emit(name, content, pngWidth) {
  const output = path.join(brandDir, name);
  await writeFile(output, content);
  if (pngWidth) {
    await sharp(Buffer.from(content)).resize({ width: pngWidth }).png().toFile(output.replace(/\.svg$/, ".png"));
  }
}

for (const theme of ["light", "dark"]) {
  const body = await markBody(theme);
  const beastColor = theme === "dark" ? "#F3F7F3" : "#151A16";
  const dbColor = theme === "dark" ? "#A8F21A" : "#82C900";
  const icon = (x, y, w, h) => `<svg x="${x}" y="${y}" width="${w}" height="${h}" viewBox="0 0 320 300">${body}</svg>`;
  await emit(
    `beastdb-horizontal-${theme}.svg`,
    svg(`BeastDB horizontal logo for ${theme} backgrounds`, "0 0 940 300",
      `${icon(10, 10, 300, 281)}\n${wordmark(315, 197, 142, beastColor, dbColor)}`),
    1590,
  );
  const horizontalPng = path.join(brandDir, `beastdb-horizontal-${theme}.png`);
  const { width, height } = await sharp(horizontalPng).metadata();
  await sharp({ create: {
    width, height, channels: 4,
    background: theme === "dark" ? "#101611" : "#F7F9F6",
  } }).composite([{ input: horizontalPng }]).png()
    .toFile(path.join(brandDir, `beastdb-preview-${theme}.png`));
  await emit(
    `beastdb-stacked-${theme}.svg`,
    svg(`BeastDB stacked logo for ${theme} backgrounds`, "0 0 760 540",
      `${icon(226, 14, 308, 289)}\n${wordmark(110, 455, 139, beastColor, dbColor)}`),
    1140,
  );
  await sharp(path.join(brandDir, `beastdb-mark-${theme}.svg`))
    .resize({ width: 512 }).png().toFile(path.join(brandDir, `beastdb-mark-${theme}.png`));
}

for (const [name, color] of [["black", "#151A16"], ["white", "#FFFFFF"]]) {
  const body = await markBody(name === "black" ? "light" : "dark");
  const paths = [...body.matchAll(/<path d="([^"]+)"/g)].map((match) => match[1]);
  const mono = `<defs><mask id="cutouts">
      <rect width="320" height="300" fill="white"/>
      <ellipse cx="160" cy="55" rx="126" ry="43" fill="none" stroke="black" stroke-width="4"/>
      ${paths.slice(1, 4).map((d) => `<path d="${d}" fill="none" stroke="black" stroke-width="5"/>`).join("\n")}
      ${paths.slice(4).map((d) => `<path d="${d}" fill="black"/>`).join("\n")}
    </mask></defs><path d="${paths[0]}" fill="${color}" mask="url(#cutouts)"/>`;
  await emit(
    `beastdb-horizontal-mono-${name}.svg`,
    svg(`BeastDB monochrome ${name} logo`, "0 0 940 300",
      `<svg x="10" y="10" width="300" height="281" viewBox="0 0 320 300">${mono}</svg>\n${wordmark(315, 197, 142, color, color)}`),
    1590,
  );
}

const favicon = svg("BeastDB favicon", "0 0 64 64", `
  <path d="M6 14c0-6 11-10 26-10s26 4 26 10v38c0 6-11 10-26 10S6 58 6 52V14Z" fill="#151A16"/>
  <ellipse cx="32" cy="14" rx="26" ry="10" fill="#273029"/>
  <path d="M6 27c1 5 12 8 26 8s25-3 26-8M6 41c1 5 12 8 26 8s25-3 26-8" fill="none" stroke="#3A453C" stroke-width="2"/>
  <path d="M46 5 14 41 23 36 54 5ZM56 19 18 62 28 57 62 19ZM63 39 42 63 51 60 64 46Z" fill="#A8F21A"/>
`);
await emit("beastdb-favicon.svg", favicon, 256);
await writeFile(path.resolve(brandDir, "../../src/app/icon.svg"), favicon);

console.log("Generated BeastDB logo assets in", brandDir);
