# regression tools

This repository contains tools for regression testing.

## Back And Recovery

### Back

It will create a backup of the database and the files in /var/lib/{component}/{version}.back.

It will have some steps:

1. The tools will annotate all component with runmode=debug.
2. The tools will exec shell to kill 1 to stop component. The order will TiDB, PD, TiKV.
3. The tools will cp the files in /var/lib/{component} exclude back to /var/lib/{component}/{version}.back.
4. The tools will restart all pods. Notion: Pods will remove all runmode annotation after pods restart.

### Recovery

It will recover the database and the files in /var/lib/{component}/{version}.back.

it will have some steps:

1. The tools will annotate all component with runmode=debug.
2. The tools will exec shell to kill 1 to stop component. The order will TiDB, PD, TiKV.
3. The tools will cp the files in /var/lib/{component}/{version}.back to /var/lib/{component}/.
4. The tools will restart all pods. Notion: Pods will remove all runmode annotation after pods restart.