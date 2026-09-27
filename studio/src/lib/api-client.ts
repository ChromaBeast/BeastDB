export async function api<T>(url: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(url, init);
  } catch {
    throw new Error("Cannot reach BeastDB. Check your connection and retry.");
  }
  if (response.status === 401) {
    throw new Error("Your session expired. Sign in again.");
  }
  if (response.status === 403) {
    throw new Error("Your account cannot make this change.");
  }
  if (response.status === 404) {
    throw new Error(
      url.startsWith("/api/key?")
        ? "That record was not found."
        : "Data endpoint unavailable.",
    );
  }
  if (!response.ok) {
    throw new Error((await response.text()).trim() || "Request failed.");
  }
  return response.status === 204 || response.status === 201
    ? (undefined as T)
    : response.json();
}
