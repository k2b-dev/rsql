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

  test("paginates namespace listings explicitly", async () => {
    const urls: string[] = [];
    const client = createRsqlClient({
      url: "https://rsql.example",
      token: "secret",
      fetch: (async (input: RequestInfo | URL) => {
        const url = String(input);
        urls.push(url);
        if (url.includes("cursor=bravo")) {
          return Response.json({ data: [{ name: "charlie" }] });
        }
        return Response.json({
          data: [{ name: "alpha" }, { name: "bravo" }],
          next_cursor: "bravo",
        });
      }) as unknown as typeof fetch,
    });

    const page = await client.namespaces.list({ limit: 2, cursor: "alpha" });
    expect(page.ok).toBe(true);
    if (page.ok) {
      expect(page.data.next_cursor).toBe("bravo");
      expect(page.data.data).toHaveLength(2);
    }
    expect(urls[0]).toBe("https://rsql.example/v1/namespaces?limit=2&cursor=alpha");

    const all = await client.namespaces.listAll();
    expect(all.ok).toBe(true);
    if (all.ok) {
      expect(all.data.map((entry) => entry.name)).toEqual(["alpha", "bravo", "charlie"]);
    }
  });
});
