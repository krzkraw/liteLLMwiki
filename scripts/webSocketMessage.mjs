export async function webSocketMessageToText(message) {
  if (typeof message === "string") {
    return message;
  }

  if (message instanceof ArrayBuffer) {
    return Buffer.from(message).toString("utf8");
  }

  if (ArrayBuffer.isView(message)) {
    return Buffer.from(
      message.buffer,
      message.byteOffset,
      message.byteLength,
    ).toString("utf8");
  }

  if (typeof Blob !== "undefined" && message instanceof Blob) {
    return await message.text();
  }

  return String(message);
}
