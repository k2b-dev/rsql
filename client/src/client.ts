import { createHttpClient } from "./http.js";
import { createNamespaceModule } from "./modules/namespace.js";
import { createNamespacesModule } from "./modules/namespaces.js";
import type { CreateRsqlClientOptions, RsqlClient } from "./types.js";

export const createRsqlClient = (options: CreateRsqlClientOptions): RsqlClient => {
  const http = createHttpClient(options);

  return {
    namespaces: createNamespacesModule(http),
    ns(name: string) {
      return createNamespaceModule(http, name);
    },
  };
};
