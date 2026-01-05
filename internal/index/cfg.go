package index

import (
	"fmt"
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/abramin/flowlens/internal/config"
	"github.com/abramin/flowlens/internal/store"
)

// InstructionInfo represents a single SSA instruction in a basic block.
type InstructionInfo struct {
	Index    int    `json:"index"`
	Op       string `json:"op"`        // e.g., "call", "if", "return", "store", etc.
	Text     string `json:"text"`      // Human-readable representation
	CalleeID *int64 `json:"callee_id"` // If this is a call, the callee symbol ID
}

// BasicBlockInfo represents a basic block in the CFG.
type BasicBlockInfo struct {
	Index        int               `json:"index"`
	Instructions []InstructionInfo `json:"instructions"`
	Succs        []int             `json:"successors"`
	Preds        []int             `json:"predecessors"`
	IsEntry      bool              `json:"is_entry"`
	IsExit       bool              `json:"is_exit"`
	BranchCond   string            `json:"branch_cond,omitempty"` // "if err != nil", "return", etc.
}

// CFGInfo represents the control flow graph for a function.
type CFGInfo struct {
	SymbolID   int64            `json:"symbol_id"`
	Name       string           `json:"name"`
	Signature  string           `json:"signature,omitempty"`
	Blocks     []BasicBlockInfo `json:"blocks"`
	EntryBlock int              `json:"entry_block"`
	ExitBlocks []int            `json:"exit_blocks"`
}

// CFGBuilder builds control flow graphs from SSA.
type CFGBuilder struct {
	st *store.Store
}

// NewCFGBuilder creates a new CFG builder.
func NewCFGBuilder(st *store.Store) *CFGBuilder {
	return &CFGBuilder{
		st: st,
	}
}

// BuildCFG constructs the CFG for a given symbol.
// This rebuilds SSA on-demand, which may take 1-2 seconds on first call.
func (cb *CFGBuilder) BuildCFG(symbolID store.SymbolID) (*CFGInfo, error) {
	// Get symbol info
	sym, err := cb.st.GetSymbolByID(symbolID)
	if err != nil {
		return nil, fmt.Errorf("symbol not found: %w", err)
	}

	// Get package path
	pkg, err := cb.st.GetPackageByPath(sym.PkgPath)
	if err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Load default config
	cfg := config.Default()

	// Create loader and load packages
	loader := NewLoader(cfg, pkg.Dir)
	if err := loader.Load(); err != nil {
		return nil, fmt.Errorf("failed to load package: %w", err)
	}

	// Build SSA program
	prog, _ := ssautil.AllPackages(loader.pkgs, ssa.SanityCheckFunctions)
	prog.Build()

	// Find the SSA function
	ssaFunc := cb.findSSAFunction(prog, sym)
	if ssaFunc == nil {
		return nil, fmt.Errorf("SSA function not found for %s", sym.Name)
	}

	// Build the CFG
	return cb.buildCFGFromSSA(symbolID, ssaFunc)
}

// findSSAFunction locates the SSA function for a symbol.
func (cb *CFGBuilder) findSSAFunction(prog *ssa.Program, sym *store.Symbol) *ssa.Function {
	for _, pkg := range prog.AllPackages() {
		if pkg.Pkg.Path() != sym.PkgPath {
			continue
		}

		// Check package-level functions
		for _, member := range pkg.Members {
			if fn, ok := member.(*ssa.Function); ok {
				if fn.Name() == sym.Name && sym.RecvType == "" {
					return fn
				}
			}

			// Check methods on named types
			if t, ok := member.(*ssa.Type); ok {
				// Check methods on the type itself
				named, ok := t.Type().(*types.Named)
				if !ok {
					continue
				}

				for i := 0; i < named.NumMethods(); i++ {
					m := named.Method(i)
					if m.Name() == sym.Name {
						fn := prog.FuncValue(m)
						if fn != nil && matchesRecvType(fn, sym.RecvType) {
							return fn
						}
					}
				}

				// Check methods on pointer to type
				ptr := types.NewPointer(named)
				mset := types.NewMethodSet(ptr)
				for i := 0; i < mset.Len(); i++ {
					sel := mset.At(i)
					if sel.Obj().Name() == sym.Name {
						fn := prog.MethodValue(sel)
						if fn != nil && matchesRecvType(fn, sym.RecvType) {
							return fn
						}
					}
				}
			}
		}
	}
	return nil
}

// matchesRecvType checks if the function's receiver matches the expected type.
func matchesRecvType(fn *ssa.Function, recvType string) bool {
	if fn.Signature.Recv() == nil {
		return recvType == ""
	}

	recv := fn.Signature.Recv().Type()
	recvStr := types.TypeString(recv, nil)

	// Handle pointer types
	if ptr, ok := recv.(*types.Pointer); ok {
		recvStr = ptr.Elem().String()
	}

	// Compare type names (strip package path)
	if idx := strings.LastIndex(recvStr, "."); idx >= 0 {
		recvStr = recvStr[idx+1:]
	}

	return recvStr == recvType || "*"+recvStr == recvType
}

// buildCFGFromSSA constructs CFGInfo from an SSA function.
func (cb *CFGBuilder) buildCFGFromSSA(symbolID store.SymbolID, fn *ssa.Function) (*CFGInfo, error) {
	if len(fn.Blocks) == 0 {
		return nil, fmt.Errorf("function has no basic blocks (may be external)")
	}

	cfg := &CFGInfo{
		SymbolID:   int64(symbolID),
		Name:       fn.Name(),
		Signature:  fn.Signature.String(),
		EntryBlock: 0,
	}

	// Process each basic block
	for _, block := range fn.Blocks {
		blockInfo := BasicBlockInfo{
			Index:   block.Index,
			IsEntry: block.Index == 0,
		}

		// Get predecessors
		for _, pred := range block.Preds {
			blockInfo.Preds = append(blockInfo.Preds, pred.Index)
		}

		// Get successors
		for _, succ := range block.Succs {
			blockInfo.Succs = append(blockInfo.Succs, succ.Index)
		}

		// Check if this is an exit block
		if len(block.Succs) == 0 {
			blockInfo.IsExit = true
			cfg.ExitBlocks = append(cfg.ExitBlocks, block.Index)
		}

		// Process instructions
		for i, instr := range block.Instrs {
			instrInfo := cb.processInstruction(instr, i)
			blockInfo.Instructions = append(blockInfo.Instructions, instrInfo)

			// Extract branch condition from last instruction
			if i == len(block.Instrs)-1 {
				blockInfo.BranchCond = cb.extractBranchCondition(instr)
			}
		}

		cfg.Blocks = append(cfg.Blocks, blockInfo)
	}

	return cfg, nil
}

// processInstruction converts an SSA instruction to InstructionInfo.
func (cb *CFGBuilder) processInstruction(instr ssa.Instruction, index int) InstructionInfo {
	info := InstructionInfo{
		Index: index,
		Op:    instrOpName(instr),
		Text:  instr.String(),
	}

	// Check for call instructions
	switch v := instr.(type) {
	case *ssa.Call:
		info.Op = "call"
		if callee := v.Call.StaticCallee(); callee != nil {
			info.Text = formatCall(v)
			// Try to resolve callee ID
			if id := cb.resolveCalleeID(callee); id != nil {
				info.CalleeID = id
			}
		} else {
			info.Text = formatCall(v)
		}

	case *ssa.Go:
		info.Op = "go"
		info.Text = "go " + formatCallCommon(&v.Call)

	case *ssa.Defer:
		info.Op = "defer"
		info.Text = "defer " + formatCallCommon(&v.Call)

	case *ssa.Return:
		info.Op = "return"
		if len(v.Results) > 0 {
			var results []string
			for _, r := range v.Results {
				results = append(results, r.Name())
			}
			info.Text = "return " + strings.Join(results, ", ")
		} else {
			info.Text = "return"
		}

	case *ssa.If:
		info.Op = "if"
		info.Text = "if " + v.Cond.Name()

	case *ssa.Panic:
		info.Op = "panic"
		info.Text = "panic(" + v.X.Name() + ")"

	case *ssa.Jump:
		info.Op = "jump"
		info.Text = "jump"
	}

	return info
}

// instrOpName returns a human-readable operation name for an instruction.
func instrOpName(instr ssa.Instruction) string {
	switch instr.(type) {
	case *ssa.Alloc:
		return "alloc"
	case *ssa.BinOp:
		return "binop"
	case *ssa.Call:
		return "call"
	case *ssa.ChangeInterface:
		return "convert"
	case *ssa.ChangeType:
		return "convert"
	case *ssa.Convert:
		return "convert"
	case *ssa.Defer:
		return "defer"
	case *ssa.Extract:
		return "extract"
	case *ssa.Field:
		return "field"
	case *ssa.FieldAddr:
		return "fieldaddr"
	case *ssa.Go:
		return "go"
	case *ssa.If:
		return "if"
	case *ssa.Index:
		return "index"
	case *ssa.IndexAddr:
		return "indexaddr"
	case *ssa.Jump:
		return "jump"
	case *ssa.Lookup:
		return "lookup"
	case *ssa.MakeChan:
		return "makechan"
	case *ssa.MakeClosure:
		return "closure"
	case *ssa.MakeInterface:
		return "makeinterface"
	case *ssa.MakeMap:
		return "makemap"
	case *ssa.MakeSlice:
		return "makeslice"
	case *ssa.MapUpdate:
		return "mapupdate"
	case *ssa.Next:
		return "next"
	case *ssa.Panic:
		return "panic"
	case *ssa.Phi:
		return "phi"
	case *ssa.Range:
		return "range"
	case *ssa.Return:
		return "return"
	case *ssa.RunDefers:
		return "rundefers"
	case *ssa.Select:
		return "select"
	case *ssa.Send:
		return "send"
	case *ssa.Slice:
		return "slice"
	case *ssa.Store:
		return "store"
	case *ssa.TypeAssert:
		return "typeassert"
	case *ssa.UnOp:
		return "unop"
	default:
		return "unknown"
	}
}

// formatCall formats a call instruction for display.
func formatCall(call *ssa.Call) string {
	return formatCallCommon(&call.Call)
}

// formatCallCommon formats a CallCommon for display.
func formatCallCommon(call *ssa.CallCommon) string {
	var sb strings.Builder

	if call.IsInvoke() {
		sb.WriteString(call.Value.Name())
		sb.WriteString(".")
		sb.WriteString(call.Method.Name())
	} else if callee := call.StaticCallee(); callee != nil {
		if recv := callee.Signature.Recv(); recv != nil {
			// Method call
			recvType := types.TypeString(recv.Type(), nil)
			if ptr, ok := recv.Type().(*types.Pointer); ok {
				recvType = ptr.Elem().String()
			}
			if idx := strings.LastIndex(recvType, "."); idx >= 0 {
				recvType = recvType[idx+1:]
			}
			sb.WriteString("(")
			sb.WriteString(recvType)
			sb.WriteString(").")
		}
		sb.WriteString(callee.Name())
	} else {
		// Dynamic call
		sb.WriteString(call.Value.Name())
	}

	sb.WriteString("(")
	for i, arg := range call.Args {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(arg.Name())
	}
	sb.WriteString(")")

	return sb.String()
}

// extractBranchCondition extracts a human-readable branch condition.
func (cb *CFGBuilder) extractBranchCondition(instr ssa.Instruction) string {
	switch v := instr.(type) {
	case *ssa.If:
		return cb.formatCondition(v.Cond)
	case *ssa.Return:
		return "return"
	case *ssa.Panic:
		return "panic"
	case *ssa.Jump:
		return ""
	default:
		return ""
	}
}

// formatCondition formats a condition value for display.
func (cb *CFGBuilder) formatCondition(cond ssa.Value) string {
	switch v := cond.(type) {
	case *ssa.BinOp:
		// Try to format as "err != nil" style
		left := cb.formatValue(v.X)
		right := cb.formatValue(v.Y)
		op := v.Op.String()
		return left + " " + op + " " + right

	case *ssa.UnOp:
		if v.Op.String() == "!" {
			return "!" + cb.formatValue(v.X)
		}
		return cb.formatValue(v)

	default:
		return cb.formatValue(cond)
	}
}

// formatValue formats a value for display.
func (cb *CFGBuilder) formatValue(v ssa.Value) string {
	if v == nil {
		return "nil"
	}

	switch val := v.(type) {
	case *ssa.Const:
		if val.Value == nil {
			return "nil"
		}
		return val.Value.String()

	case *ssa.Extract:
		// Often used for error values: t0, err := ...
		if val.Tuple != nil {
			if call, ok := val.Tuple.(*ssa.Call); ok {
				results := call.Type().(*types.Tuple)
				if val.Index < results.Len() {
					name := results.At(val.Index).Name()
					if name != "" {
						return name
					}
				}
			}
		}
		return val.Name()

	case *ssa.Phi:
		return val.Name()

	default:
		name := v.Name()
		if name != "" {
			return name
		}
		return v.String()
	}
}

// resolveCalleeID tries to find the store symbol ID for an SSA function.
func (cb *CFGBuilder) resolveCalleeID(callee *ssa.Function) *int64 {
	if callee == nil || callee.Pkg == nil {
		return nil
	}

	pkgPath := callee.Pkg.Pkg.Path()
	name := callee.Name()

	var recvType string
	if recv := callee.Signature.Recv(); recv != nil {
		recvType = types.TypeString(recv.Type(), nil)
		if ptr, ok := recv.Type().(*types.Pointer); ok {
			recvType = ptr.Elem().String()
		}
		if idx := strings.LastIndex(recvType, "."); idx >= 0 {
			recvType = recvType[idx+1:]
		}
	}

	// Try to find symbol in store
	id, err := cb.st.FindSymbolID(pkgPath, name, recvType)
	if err != nil {
		return nil
	}

	idInt64 := int64(id)
	return &idInt64
}
