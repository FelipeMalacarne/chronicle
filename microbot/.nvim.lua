-- Loaded by Neovim's 'exrc'. jdtls comes from the flake's dev shell (direnv).
if vim.fn.executable("jdtls") == 1 then
	vim.lsp.enable("jdtls")
end
