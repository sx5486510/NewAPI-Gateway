# CPA Authentication ZIP Upload Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the CPA authentication-file page upload ZIP archives whose nested JSON files are safely expanded and forwarded through CPA's existing authentication-file API.

**Architecture:** Keep ZIP processing in `ManagementProxy`, which already intercepts `POST /v0/management/auth-files`. Extend its multipart parser to distinguish supported JSON/ZIP parts, share a ZIP expansion budget across the request, reject archives that yield no JSON, and forward only flattened JSON files to CPA. The React component only broadens file selection and communicates the server behavior; it does not parse credentials or archives.

**Tech Stack:** Go `archive/zip`, `mime/multipart`, `io`; React 18; Jest; Go `testing` and `httptest`.

---

### Task 1: Reject ZIP uploads that cannot produce authentication files

**Files:**
- Modify: `service/cpa/management_proxy_test.go`
- Modify: `service/cpa/management_proxy.go`

- [ ] **Step 1: Write failing request-level tests**

Add a table-driven test which posts an empty ZIP, a ZIP containing only `notes.txt`, a damaged ZIP, and a top-level `notes.txt`. Use an upstream handler which increments a call counter and fails the test if it receives any request:

```go
func TestManagementProxyRejectsInvalidAuthFileArchives(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		body     func(*testing.T) []byte
	}{
		{"empty zip", "empty.zip", func(t *testing.T) []byte { return buildAuthFilesZip(t, nil) }},
		{"zip without json", "notes.zip", func(t *testing.T) []byte {
			return buildAuthFilesZip(t, map[string]string{"nested/notes.txt": "ignored"})
		}},
		{"damaged zip", "damaged.zip", func(*testing.T) []byte { return []byte("not a zip") }},
		{"unsupported top-level file", "notes.txt", func(*testing.T) []byte { return []byte("not json") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
			}))
			defer upstream.Close()

			upstreamURL, _ := url.Parse(upstream.URL)
			proxy := NewManagementProxy(&fakeLeaseProvider{target: upstreamURL, password: "runtime-secret"}, nil, nil)
			rec := postAuthUpload(t, proxy, tc.filename, tc.body(t))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if upstreamCalls.Load() != 0 {
				t.Fatalf("upstream calls = %d, want 0", upstreamCalls.Load())
			}
		})
	}
}
```

Add the reusable multipart request helper near `buildAuthFilesZip`:

```go
func postAuthUpload(t *testing.T, proxy http.Handler, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil { t.Fatal(err) }
	if _, err := part.Write(content); err != nil { t.Fatal(err) }
	if err := writer.Close(); err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodPost, "/v0/management/auth-files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)
	return rec
}
```

- [ ] **Step 2: Run the new test and verify red**

Run: `go test ./service/cpa -run TestManagementProxyRejectsInvalidAuthFileArchives -count=1`

Expected: FAIL because empty/no-JSON ZIP uploads fall through to CPA and top-level non-JSON files are accepted.

- [ ] **Step 3: Track supported multipart parts explicitly**

Update `uploadedAuthFiles` so it sets `hasFilePart` for every multipart `file`, accepts direct `.json`, expands `.zip`, rejects other extensions, and errors when recognized parts yield no JSON:

```go
func uploadedAuthFiles(r *http.Request) ([]authUploadFile, bool, error) {
	// Preserve the existing media-type, boundary, and body replay setup.
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var files []authUploadFile
	budget := authZipBudget{}
	hasFilePart := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			if hasFilePart && len(files) == 0 {
				return nil, true, errors.New("upload contains no JSON authentication files")
			}
			return files, hasFilePart, nil
		}
		if err != nil { return nil, false, err }
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}

		hasFilePart = true
		filename := part.FileName()
		partBody, readErr := io.ReadAll(part)
		_ = part.Close()
		if readErr != nil { return nil, true, readErr }

		switch {
		case strings.EqualFold(path.Ext(filename), ".json"):
			files = append(files, authUploadFile{Name: filename, Body: partBody})
		case isZipUpload(filename):
			zipFiles, zipErr := authFilesFromZip(filename, partBody, &budget)
			if zipErr != nil { return nil, true, zipErr }
			files = append(files, zipFiles...)
		default:
			return nil, true, fmt.Errorf("unsupported authentication file type: %s", filename)
		}
	}
}
```

- [ ] **Step 4: Run the invalid-archive test and verify green**

Run: `go test ./service/cpa -run TestManagementProxyRejectsInvalidAuthFileArchives -count=1`

Expected: PASS with no upstream calls.

- [ ] **Step 5: Commit the behavior fix**

```bash
git add service/cpa/management_proxy.go service/cpa/management_proxy_test.go
git commit -m "fix(cpa): reject invalid auth archives"
```

### Task 2: Enforce ZIP file count and decompression limits

**Files:**
- Modify: `service/cpa/management_proxy_test.go`
- Modify: `service/cpa/management_proxy.go`

- [ ] **Step 1: Add focused failing limit tests**

Add tests which call the archive parser directly. Build 10,001 empty JSON entries for the count boundary, an entry one byte larger than 8 MiB, and two entries whose total exceeds 64 MiB:

```go
func TestAuthFilesFromZipEnforcesExpansionLimits(t *testing.T) {
	t.Run("file count", func(t *testing.T) {
		files := make(map[string]string, maxAuthFilesFromZip+1)
		for i := 0; i <= maxAuthFilesFromZip; i++ {
			files[fmt.Sprintf("nested/%05d.json", i)] = ""
		}
		_, err := authFilesFromZip("many.zip", buildAuthFilesZip(t, files), &authZipBudget{})
		if err == nil || !strings.Contains(err.Error(), "10,000") {
			t.Fatalf("err = %v, want file-count error", err)
		}
	})

	t.Run("single file size", func(t *testing.T) {
		archive := buildAuthFilesZip(t, map[string]string{"large.json": strings.Repeat("x", int(maxAuthFileFromZipBytes)+1)})
		_, err := authFilesFromZip("large.zip", archive, &authZipBudget{})
		if err == nil || !strings.Contains(err.Error(), "8 MiB") {
			t.Fatalf("err = %v, want per-file size error", err)
		}
	})

	t.Run("combined size", func(t *testing.T) {
		chunk := strings.Repeat("x", int(maxAuthFilesFromZipBytes/2)+1)
		archive := buildAuthFilesZip(t, map[string]string{"a.json": chunk, "b.json": chunk})
		_, err := authFilesFromZip("total.zip", archive, &authZipBudget{})
		if err == nil || !strings.Contains(err.Error(), "64 MiB") {
			t.Fatalf("err = %v, want total-size error", err)
		}
	})
}
```

- [ ] **Step 2: Run the focused test and verify red**

Run: `go test ./service/cpa -run TestAuthFilesFromZipEnforcesExpansionLimits -count=1`

Expected: FAIL because constants, budget state, and bounded reads do not exist.

- [ ] **Step 3: Add shared request budget and bounded archive reads**

Define limits beside the existing request-size constant:

```go
const (
	maxAuthFilesFromZip      = 10_000
	maxAuthFileFromZipBytes  = int64(8 << 20)
	maxAuthFilesFromZipBytes = int64(64 << 20)
)

type authZipBudget struct {
	files int
	bytes int64
}
```

Change `authFilesFromZip` to receive `*authZipBudget`. Normalize archive separators, count every JSON candidate, preflight declared sizes, and use a bounded reader even after preflight:

```go
func authFilesFromZip(filename string, body []byte, budget *authZipBudget) ([]authUploadFile, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil { return nil, fmt.Errorf("invalid zip %s: %w", filename, err) }

	var files []authUploadFile
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() { continue }
		name := path.Base(strings.ReplaceAll(entry.Name, "\\", "/"))
		if name == "." || name == "/" || name == "" || !strings.EqualFold(path.Ext(name), ".json") { continue }
		if budget.files >= maxAuthFilesFromZip {
			return nil, errors.New("ZIP upload contains more than 10,000 JSON files")
		}
		budget.files++
		if entry.UncompressedSize64 > uint64(maxAuthFileFromZipBytes) {
			return nil, fmt.Errorf("ZIP entry %s exceeds 8 MiB", entry.Name)
		}
		remaining := maxAuthFilesFromZipBytes - budget.bytes
		if entry.UncompressedSize64 > uint64(remaining) {
			return nil, errors.New("ZIP JSON content exceeds 64 MiB")
		}

		rc, err := entry.Open()
		if err != nil { return nil, fmt.Errorf("read zip entry %s: %w", entry.Name, err) }
		entryBody, readErr := io.ReadAll(io.LimitReader(rc, minInt64(maxAuthFileFromZipBytes, remaining)+1))
		closeErr := rc.Close()
		if readErr != nil { return nil, fmt.Errorf("read zip entry %s: %w", entry.Name, readErr) }
		if closeErr != nil { return nil, fmt.Errorf("close zip entry %s: %w", entry.Name, closeErr) }
		if int64(len(entryBody)) > maxAuthFileFromZipBytes { return nil, fmt.Errorf("ZIP entry %s exceeds 8 MiB", entry.Name) }
		if int64(len(entryBody)) > remaining { return nil, errors.New("ZIP JSON content exceeds 64 MiB") }
		budget.bytes += int64(len(entryBody))
		files = append(files, authUploadFile{Name: name, Body: entryBody})
	}
	return files, nil
}

func minInt64(a, b int64) int64 {
	if a < b { return a }
	return b
}
```

- [ ] **Step 4: Run ZIP and upload regression tests**

Run: `go test ./service/cpa -run 'Test(AuthFilesFromZip|ManagementProxy.*AuthFile)' -count=1`

Expected: PASS, including recursive nested JSON import and duplicate skipping.

- [ ] **Step 5: Commit the ZIP safety limits**

```bash
git add service/cpa/management_proxy.go service/cpa/management_proxy_test.go
git commit -m "fix(cpa): bound auth zip expansion"
```

### Task 3: Expose ZIP upload in the CPA page

**Files:**
- Modify: `web/src/components/CPAAuthFiles.test.js`
- Modify: `web/src/components/CPAAuthFiles.js`

- [ ] **Step 1: Write failing UI assertions**

Extend `opens upload modal when upload button clicked`:

```js
const fileInput = container.querySelector('input[type="file"]');
expect(fileInput.accept).toContain('.json');
expect(fileInput.accept).toContain('.zip');
expect(container.textContent).toContain('递归扫描 ZIP 子目录');
expect(container.textContent).toContain('只导入 JSON 文件');
```

Add a request test proving a selected ZIP is appended unchanged for server-side processing:

```js
test('uploads zip archives for server-side expansion', async () => {
  mockCPAAuthGet();
  helpers.API.post.mockResolvedValueOnce({ data: { success: true, uploaded: ['nested.json'], duplicates: [] } });
  await act(async () => {
    createRoot(container).render(<CPAAuthFiles />);
    await waitForUI();
  });
  await act(async () => { findButton(container, '上传认证文件').click(); });

  const fileInput = container.querySelector('input[type="file"]');
  const archive = new File(['zip-content'], 'accounts.zip', { type: 'application/zip' });
  await act(async () => {
    Object.defineProperty(fileInput, 'files', { value: [archive], configurable: true });
    fileInput.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await act(async () => {
    Array.from(container.querySelectorAll('button')).pop().click();
    await waitForUI();
  });

  const formData = helpers.API.post.mock.calls[0][1];
  expect(formData.getAll('file').map((file) => file.name)).toEqual(['accounts.zip']);
});
```

- [ ] **Step 2: Run the frontend test and verify red**

Run from `web`: `npm test -- --runInBand --watchAll=false CPAAuthFiles.test.js`

Expected: FAIL because `.zip` and the ZIP behavior text are absent.

- [ ] **Step 3: Broaden the input and update its concise help text**

Change the upload input and hint in `CPAAuthFiles.js`:

```jsx
<input
  type='file'
  multiple
  accept='.json,.zip,application/json,application/zip'
  onChange={(e) => setUploadFiles(Array.from(e.target.files || []))}
  style={{ width: '100%' }}
/>
<p style={{ marginTop: '0.5rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
  支持多个 JSON 或 ZIP；递归扫描 ZIP 子目录，只导入 JSON 文件
</p>
```

- [ ] **Step 4: Run the component test and verify green**

Run from `web`: `npm test -- --runInBand --watchAll=false CPAAuthFiles.test.js`

Expected: PASS.

- [ ] **Step 5: Commit the frontend support**

```bash
git add web/src/components/CPAAuthFiles.js web/src/components/CPAAuthFiles.test.js
git commit -m "feat(cpa): allow zip auth uploads"
```

### Task 4: Full verification

**Files:**
- No file changes expected.

- [ ] **Step 1: Format and verify the Go package**

Run: `gofmt -w service/cpa/management_proxy.go service/cpa/management_proxy_test.go`

Run: `go test ./service/cpa -count=1`

Expected: PASS.

- [ ] **Step 2: Run all frontend tests**

Run from `web`: `npm test -- --runInBand --watchAll=false`

Expected: all suites pass.

- [ ] **Step 3: Build the production frontend**

Run from `web`: `npm run build`

Expected: production build succeeds without compile errors.

- [ ] **Step 4: Check the final diff**

Run: `git diff --check HEAD~3..HEAD`

Expected: no whitespace errors; only the ZIP design, plan, backend, and frontend files are included.
