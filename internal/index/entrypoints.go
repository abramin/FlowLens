package index

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/abramin/flowlens/internal/store"
	"golang.org/x/tools/go/packages"
)

// EntrypointDetector detects program entrypoints from AST.
type EntrypointDetector struct {
	loader *Loader
	fset   *token.FileSet
}

// NewEntrypointDetector creates a new entrypoint detector.
func NewEntrypointDetector(loader *Loader) *EntrypointDetector {
	return &EntrypointDetector{
		loader: loader,
		fset:   loader.FileSet(),
	}
}

// HTTPMeta holds metadata for HTTP entrypoints.
type HTTPMeta struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// GRPCMeta holds metadata for gRPC entrypoints.
type GRPCMeta struct {
	Service string `json:"service"`
	Method  string `json:"method"`
}

// CLIMeta holds metadata for CLI entrypoints.
type CLIMeta struct {
	Command   string `json:"command"`
	Parent    string `json:"parent,omitempty"`
	UsesRunE  bool   `json:"uses_run_e,omitempty"`
}

// DetectResult holds the results of entrypoint detection.
type DetectResult struct {
	HTTPCount  int
	GRPCCount  int
	CLICount   int
	MainCount  int
	TotalCount int
}

// Detect finds all entrypoints and persists them to the database.
func (d *EntrypointDetector) Detect(batch *store.BatchTx) (*DetectResult, error) {
	result := &DetectResult{}

	for _, pkg := range d.loader.Packages() {
		for i, file := range pkg.Syntax {
			goFile := pkg.GoFiles[i]
			if d.loader.shouldExcludeFile(goFile) {
				continue
			}

			// Detect HTTP entrypoints
			httpEPs, err := d.detectHTTP(pkg, file, goFile, batch)
			if err != nil {
				return nil, fmt.Errorf("detecting HTTP entrypoints in %s: %w", goFile, err)
			}
			result.HTTPCount += httpEPs

			// Detect gRPC entrypoints
			grpcEPs, err := d.detectGRPC(pkg, file, goFile, batch)
			if err != nil {
				return nil, fmt.Errorf("detecting gRPC entrypoints in %s: %w", goFile, err)
			}
			result.GRPCCount += grpcEPs

			// Detect Cobra CLI entrypoints
			cliEPs, err := d.detectCobra(pkg, file, goFile, batch)
			if err != nil {
				return nil, fmt.Errorf("detecting CLI entrypoints in %s: %w", goFile, err)
			}
			result.CLICount += cliEPs

			// Detect main() entrypoints
			mainEPs, err := d.detectMain(pkg, file, goFile, batch)
			if err != nil {
				return nil, fmt.Errorf("detecting main entrypoints in %s: %w", goFile, err)
			}
			result.MainCount += mainEPs
		}
	}

	result.TotalCount = result.HTTPCount + result.GRPCCount + result.CLICount + result.MainCount
	return result, nil
}

// detectHTTP finds HTTP route registrations (stdlib, chi, gin).
func (d *EntrypointDetector) detectHTTP(pkg *packages.Package, file *ast.File, goFile string, batch *store.BatchTx) (int, error) {
	count := 0

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Try to match different HTTP registration patterns
		var method, path string
		var handlerExpr ast.Expr

		// Check for selector expressions (e.g., mux.HandleFunc, r.Get)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			methodName := sel.Sel.Name

			switch {
			// stdlib http.HandleFunc, http.Handle, mux.HandleFunc, mux.Handle
			case methodName == "HandleFunc" || methodName == "Handle":
				if len(call.Args) >= 2 {
					path = d.extractStringLiteral(call.Args[0])
					handlerExpr = call.Args[1]
					method = "ANY" // stdlib doesn't specify method
				}

			// chi router: r.Get, r.Post, r.Put, r.Delete, r.Patch, r.Options, r.Head
			case methodName == "Get" || methodName == "Post" || methodName == "Put" ||
				methodName == "Delete" || methodName == "Patch" || methodName == "Options" ||
				methodName == "Head" || methodName == "Connect" || methodName == "Trace":
				if len(call.Args) >= 2 {
					path = d.extractStringLiteral(call.Args[0])
					handlerExpr = call.Args[1]
					method = strings.ToUpper(methodName)
				}

			// chi router: r.Method
			case methodName == "Method":
				if len(call.Args) >= 3 {
					method = d.extractStringLiteral(call.Args[0])
					path = d.extractStringLiteral(call.Args[1])
					handlerExpr = call.Args[2]
				}

			// gin router: r.GET, r.POST, r.PUT, r.DELETE, etc. (uppercase)
			case methodName == "GET" || methodName == "POST" || methodName == "PUT" ||
				methodName == "DELETE" || methodName == "PATCH" || methodName == "OPTIONS" ||
				methodName == "HEAD":
				if len(call.Args) >= 2 {
					path = d.extractStringLiteral(call.Args[0])
					handlerExpr = call.Args[1]
					method = methodName
				}

			// gin router: r.Any, r.Handle
			case methodName == "Any":
				if len(call.Args) >= 2 {
					path = d.extractStringLiteral(call.Args[0])
					handlerExpr = call.Args[1]
					method = "ANY"
				}
			}
		}

		// If we found a valid route registration
		if path != "" && handlerExpr != nil {
			// Resolve handler to symbol
			symbolID := d.resolveHandlerSymbol(pkg, handlerExpr, batch)
			if symbolID != 0 {
				meta := HTTPMeta{Method: method, Path: path}
				metaJSON, _ := json.Marshal(meta)

				ep := &store.Entrypoint{
					Type:     store.EntrypointHTTP,
					Label:    fmt.Sprintf("%s %s", method, path),
					SymbolID: symbolID,
					MetaJSON: string(metaJSON),
				}

				if err := batch.InsertEntrypoint(ep); err == nil {
					count++
				}
			}
		}

		return true
	})

	return count, nil
}

// detectGRPC finds gRPC service registrations (RegisterXServer patterns).
func (d *EntrypointDetector) detectGRPC(pkg *packages.Package, file *ast.File, goFile string, batch *store.BatchTx) (int, error) {
	count := 0

	// Track registered services and their implementation types
	type registrationInfo struct {
		serviceName string
		implExpr    ast.Expr
		pkgPath     string
	}
	var registrations []registrationInfo

	// First pass: find RegisterXServer calls
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcName string
		var funcPkg string

		// Check for selector: pb.RegisterUserServiceServer(...)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			funcName = sel.Sel.Name
			if ident, ok := sel.X.(*ast.Ident); ok {
				// Get the package path from imports
				funcPkg = d.getImportPath(file, ident.Name)
			}
		}

		// Check for direct call: RegisterUserServiceServer(...)
		if ident, ok := call.Fun.(*ast.Ident); ok {
			funcName = ident.Name
			funcPkg = pkg.PkgPath
		}

		// Match RegisterXServer pattern
		if strings.HasPrefix(funcName, "Register") && strings.HasSuffix(funcName, "Server") {
			serviceName := funcName[8 : len(funcName)-6] // Extract service name
			if len(call.Args) >= 2 {
				registrations = append(registrations, registrationInfo{
					serviceName: serviceName,
					implExpr:    call.Args[1],
					pkgPath:     funcPkg,
				})
			}
		}

		return true
	})

	// For each registration, try to find the service interface methods
	for _, reg := range registrations {
		// Try to resolve the implementation type
		implType := d.resolveExprType(pkg, reg.implExpr)
		if implType == "" {
			continue
		}

		// Find methods on the implementation type that match service methods
		methods := d.findServiceMethods(pkg, implType, reg.serviceName)
		for _, methodName := range methods {
			// Look up the symbol for this method
			symbolID, err := batch.GetSymbolID(pkg.PkgPath, methodName, implType)
			if err != nil {
				// Try with pointer receiver
				symbolID, err = batch.GetSymbolID(pkg.PkgPath, methodName, "*"+implType)
			}
			if err != nil {
				continue
			}

			meta := GRPCMeta{Service: reg.serviceName, Method: methodName}
			metaJSON, _ := json.Marshal(meta)

			ep := &store.Entrypoint{
				Type:     store.EntrypointGRPC,
				Label:    fmt.Sprintf("%s/%s", reg.serviceName, methodName),
				SymbolID: symbolID,
				MetaJSON: string(metaJSON),
			}

			if err := batch.InsertEntrypoint(ep); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// detectCobra finds Cobra CLI command definitions.
func (d *EntrypointDetector) detectCobra(pkg *packages.Package, file *ast.File, goFile string, batch *store.BatchTx) (int, error) {
	count := 0

	// Track command definitions
	type commandInfo struct {
		use         string
		runHandler  ast.Expr
		runEHandler ast.Expr
		parent      string
	}
	var commands []commandInfo

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for &cobra.Command{...} composite literals
		unary, ok := n.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			return true
		}

		compLit, ok := unary.X.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Check if it's a cobra.Command type
		if !d.isCobraCommandType(compLit.Type) {
			return true
		}

		cmd := commandInfo{}
		for _, elt := range compLit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}

			switch key.Name {
			case "Use":
				cmd.use = d.extractStringLiteral(kv.Value)
			case "Run":
				cmd.runHandler = kv.Value
			case "RunE":
				cmd.runEHandler = kv.Value
			}
		}

		if cmd.use != "" && (cmd.runHandler != nil || cmd.runEHandler != nil) {
			commands = append(commands, cmd)
		}

		return true
	})

	// Insert entrypoints for each command
	for _, cmd := range commands {
		var handlerExpr ast.Expr
		usesRunE := false
		if cmd.runEHandler != nil {
			handlerExpr = cmd.runEHandler
			usesRunE = true
		} else {
			handlerExpr = cmd.runHandler
		}

		symbolID := d.resolveHandlerSymbol(pkg, handlerExpr, batch)
		if symbolID != 0 {
			// Extract command name from Use field (first word)
			cmdName := strings.Fields(cmd.use)[0]
			meta := CLIMeta{Command: cmdName, UsesRunE: usesRunE}
			metaJSON, _ := json.Marshal(meta)

			ep := &store.Entrypoint{
				Type:     store.EntrypointCLI,
				Label:    cmdName,
				SymbolID: symbolID,
				MetaJSON: string(metaJSON),
			}

			if err := batch.InsertEntrypoint(ep); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// detectMain finds main() function entrypoints.
func (d *EntrypointDetector) detectMain(pkg *packages.Package, file *ast.File, goFile string, batch *store.BatchTx) (int, error) {
	// Only look for main in main package
	if pkg.Name != "main" {
		return 0, nil
	}

	count := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		// Check for main function (no receiver, name is "main")
		if fn.Name.Name == "main" && fn.Recv == nil {
			symbolID, err := batch.GetSymbolID(pkg.PkgPath, "main", "")
			if err != nil {
				continue
			}

			ep := &store.Entrypoint{
				Type:     store.EntrypointMain,
				Label:    "main",
				SymbolID: symbolID,
			}

			if err := batch.InsertEntrypoint(ep); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// resolveHandlerSymbol attempts to resolve a handler expression to a symbol ID.
func (d *EntrypointDetector) resolveHandlerSymbol(pkg *packages.Package, expr ast.Expr, batch *store.BatchTx) store.SymbolID {
	switch e := expr.(type) {
	case *ast.Ident:
		// Simple function reference: handler
		symbolID, err := batch.GetSymbolID(pkg.PkgPath, e.Name, "")
		if err == nil {
			return symbolID
		}

	case *ast.SelectorExpr:
		// Method value: obj.Method or pkg.Func
		methodName := e.Sel.Name

		if ident, ok := e.X.(*ast.Ident); ok {
			// Try as receiver type method
			recvType := ident.Name
			symbolID, err := batch.GetSymbolID(pkg.PkgPath, methodName, recvType)
			if err == nil {
				return symbolID
			}
			symbolID, err = batch.GetSymbolID(pkg.PkgPath, methodName, "*"+recvType)
			if err == nil {
				return symbolID
			}

			// Try as package-level function from import
			importPath := d.getImportPath(nil, ident.Name)
			if importPath != "" {
				symbolID, err := batch.GetSymbolID(importPath, methodName, "")
				if err == nil {
					return symbolID
				}
			}
		}

	case *ast.FuncLit:
		// Anonymous function - we can't easily track these
		return 0
	}

	return 0
}

// extractStringLiteral extracts a string value from an expression.
func (d *EntrypointDetector) extractStringLiteral(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			// Remove quotes
			s := e.Value
			if len(s) >= 2 {
				return s[1 : len(s)-1]
			}
		}
	case *ast.Ident:
		// Could be a constant - would need to resolve
		return ""
	}
	return ""
}

// isCobraCommandType checks if a type expression refers to cobra.Command.
func (d *EntrypointDetector) isCobraCommandType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Command" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "cobra"
}

// getImportPath returns the import path for a package alias.
func (d *EntrypointDetector) getImportPath(file *ast.File, alias string) string {
	if file == nil {
		return ""
	}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
		} else {
			// Use the last component of the path
			parts := strings.Split(path, "/")
			name = parts[len(parts)-1]
		}
		if name == alias {
			return path
		}
	}
	return ""
}

// resolveExprType attempts to get the type name from an expression.
func (d *EntrypointDetector) resolveExprType(pkg *packages.Package, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return d.resolveExprType(pkg, e.X)
		}
	case *ast.CompositeLit:
		return d.resolveExprType(pkg, e.Type)
	case *ast.SelectorExpr:
		// pkg.Type
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name + "." + e.Sel.Name
		}
	}
	return ""
}

// findServiceMethods finds methods on a type that look like gRPC service methods.
// gRPC methods typically have signature: (ctx context.Context, req *Request) (*Response, error)
func (d *EntrypointDetector) findServiceMethods(pkg *packages.Package, typeName, serviceName string) []string {
	var methods []string

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}

			// Check if receiver matches the type
			recvType := formatReceiverType(fn.Recv.List[0].Type)
			if recvType != typeName && recvType != "*"+typeName {
				continue
			}

			// Check if method signature looks like a gRPC method
			// Must have at least 2 params (ctx, req) and 2 results (resp, error)
			if fn.Type.Params == nil || fn.Type.Results == nil {
				continue
			}
			if len(fn.Type.Params.List) < 2 || len(fn.Type.Results.List) < 2 {
				continue
			}

			// Skip methods that are clearly not gRPC (e.g., unexported)
			if !ast.IsExported(fn.Name.Name) {
				continue
			}

			// Skip mustEmbedUnimplemented methods
			if strings.HasPrefix(fn.Name.Name, "mustEmbedUnimplemented") {
				continue
			}

			methods = append(methods, fn.Name.Name)
		}
	}

	return methods
}
