export interface TokenResponse {
  id: string;
  label: string;
  created_at: string;
  last_used_at?: string;
}

export interface UserInfoResponse {
  username: string;
  effective_username: string;
  is_admin: boolean;
  is_impersonating: boolean;
  first_login_completed: boolean;
  tokens: TokenResponse[];
  flash_token?: string;
  quota_bytes?: number | null;
  used_bytes: number;
}

export interface FileItem {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export interface ExplorerResult {
  current_path: string;
  breadcrumbs: string[];
  items: FileItem[];
}
