import type { HttpClient } from "../http.js";
import type {
  ImportFile,
  NamespaceListOptions,
  NamespacePage,
  NamespaceRecord,
  NamespacesModule,
  NamespaceConfig,
  NamespaceDefinition,
  RsqlResult,
} from "../types.js";

const version = "v1";
const jsonHeaders: HeadersInit = {
  "Content-Type": "application/json",
};

export const createNamespacesModule = (http: HttpClient): NamespacesModule => {
  return {
    create(request: NamespaceDefinition) {
      const payload: NamespaceDefinition = {
        ...request,
        config: withDefaultNamespaceConfig(request.config),
      };
      return http.json(`/${version}/namespaces`, {
        method: "POST",
        headers: jsonHeaders,
        body: JSON.stringify(payload),
      });
    },

    list(options?: NamespaceListOptions) {
      return http.json<NamespacePage>(
        http.withQuery(`/${version}/namespaces`, {
          limit: options?.limit,
          cursor: options?.cursor,
        }),
      );
    },

    async listAll() {
      const records: NamespaceRecord[] = [];
      let cursor: string | undefined;
      let lastResult: RsqlResult<NamespacePage> | undefined;
      do {
        lastResult = await http.json<NamespacePage>(
          http.withQuery(`/${version}/namespaces`, {
            limit: 500,
            cursor,
          }),
        );
        if (!lastResult.ok) {
          return lastResult;
        }
        records.push(...lastResult.data.data);
        cursor = lastResult.data.next_cursor;
      } while (cursor);

      return {
        ok: true,
        data: records,
        status: lastResult?.status ?? 200,
        headers: lastResult?.headers ?? new Headers(),
      };
    },

    get(name: string) {
      return http.json(`/${version}/namespaces/${encodeURIComponent(name)}`);
    },

    update(name: string, config: NamespaceConfig) {
      return http.json(`/${version}/namespaces/${encodeURIComponent(name)}`, {
        method: "PUT",
        headers: jsonHeaders,
        body: JSON.stringify({ config }),
      });
    },

    delete(name: string) {
      return http.empty(`/${version}/namespaces/${encodeURIComponent(name)}`, {
        method: "DELETE",
      });
    },

    duplicate(source: string, target: string) {
      return http.json(`/${version}/namespaces/${encodeURIComponent(source)}/duplicate`, {
        method: "POST",
        headers: jsonHeaders,
        body: JSON.stringify({ name: target }),
      });
    },

    exportDb(name: string) {
      return http.raw(`/${version}/namespaces/${encodeURIComponent(name)}/export`);
    },

    importDb(name: string, file: ImportFile) {
      const formData = new FormData();
      formData.set("file", toBlob(file.content, file.contentType), file.filename || "namespace.db");

      return http.json(`/${version}/namespaces/${encodeURIComponent(name)}/import`, {
        method: "POST",
        body: formData,
      });
    },

    importCsv(name: string, table: string, file: ImportFile) {
      const formData = new FormData();
      formData.set("file", toBlob(file.content, file.contentType ?? "text/csv"), file.filename || "rows.csv");

      return http.json(`/${version}/namespaces/${encodeURIComponent(name)}/import?table=${encodeURIComponent(table)}`, {
        method: "POST",
        body: formData,
      });
    },
  };
};

const withDefaultNamespaceConfig = (config?: NamespaceConfig): NamespaceConfig => {
  const fallback: NamespaceConfig = {
    journal_mode: "wal",
    synchronous: "full",
    busy_timeout: 5000,
    query_timeout: 10000,
    foreign_keys: true,
    read_only: false,
  };

  return {
    ...fallback,
    ...(config ?? {}),
  };
};

const toBlob = (content: ImportFile["content"], contentType?: string): Blob => {
  if (content instanceof Blob) {
    return content;
  }
  if (typeof content === "string") {
    return new Blob([content], { type: contentType });
  }
  if (content instanceof Uint8Array) {
    const copy = new Uint8Array(content.byteLength);
    copy.set(content);
    return new Blob([copy.buffer], { type: contentType });
  }
  return new Blob([new Uint8Array(content)], { type: contentType });
};
