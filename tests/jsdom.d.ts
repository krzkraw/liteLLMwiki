declare module "jsdom" {
  export class JSDOM {
    constructor(
      html?: string,
      options?: {
        pretendToBeVisual?: boolean;
        url?: string;
      },
    );

    window: Window & typeof globalThis;
  }
}
