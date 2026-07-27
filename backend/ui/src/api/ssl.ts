import { request } from "./request";

export interface SSLStatus {
  enabled: boolean;
  hasCerts: boolean;
  expiry?: string;
  issuer?: string;
  selfSigned?: boolean;
}

export interface GenerateCertRequest {
  commonName?: string;
  validityDays?: number;
}

export async function fetchSSLStatus(): Promise<SSLStatus> {
  return request<SSLStatus>("/ssl/status");
}

export async function generateSSLCerts(
  req: GenerateCertRequest,
): Promise<{ status: string; message: string }> {
  return request("/ssl/generate", {
    method: "POST",
    body: JSON.stringify(req),
  });
}

export async function uploadSSLCerts(formData: FormData): Promise<{
  status: string;
  message: string;
}> {
  return request("/ssl/upload", {
    method: "POST",
    headers: {},
    body: formData,
  });
}

export async function enableSSLCerts(): Promise<{
  status: string;
  message: string;
}> {
  return request("/ssl/enable", {
    method: "POST",
  });
}

export async function disableSSLCerts(): Promise<{
  status: string;
  message: string;
}> {
  return request("/ssl/disable", {
    method: "POST",
  });
}

export async function deleteSSLCerts(): Promise<{
  status: string;
  message: string;
}> {
  return request("/ssl", {
    method: "DELETE",
  });
}

export async function downloadCACert(): Promise<Blob> {
  const response = await fetch("/api/ssl/download", {
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error("Failed to download CA certificate");
  }
  return response.blob();
}

