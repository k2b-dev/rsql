import { describe, expect, test } from "bun:test";
import { createRsqlClient } from "../src";

describe("rsql HTTP transport", () => {
  test("preserves a plain-text API error", async () => {
    const client = createRsqlClient({
      url: "https://rsql.example",
      token: "secret",
      fetch: (async () => {
        return new Response("upstream unavailable", {
          status: 502,
          statusText: "Bad Gateway",
        });
      }) as unknown as typeof fetch,
    });

    const result = await client.namespaces.list();
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.status).toBe(502);
      expect(result.error).toEqual({
        error: "unknown_error",
        message: "upstream unavailable",
      });
    }
  });

  test("does not consume namespace exports", async () => {
    let requestURL = "";
    let authorization = "";
    const client = createRsqlClient({
      url: "https://rsql.example/",
      token: "secret",
      fetch: (async (input: RequestInfo | URL, init?: RequestInit) => {
        requestURL = String(input);
        authorization = new Headers(init?.headers).get("Authorization") ?? "";
        return new Response(new Uint8Array([1, 2, 3]), {
          status: 200,
          headers: { "Content-Type": "application/octet-stream" },
        });
      }) as unknown as typeof fetch,
    });

    const result = await client.namespaces.exportDb("tenant");
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.data.bodyUsed).toBe(false);
      expect(new Uint8Array(await result.data.arrayBuffer())).toEqual(new Uint8Array([1, 2, 3]));
    }
    expect(requestURL).toBe("https://rsql.example/v1/namespaces/tenant/export");
    expect(authorization).toBe("Bearer secret");
  });
});
