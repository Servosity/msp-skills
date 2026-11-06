$n = Need-Name 'name' (Arg 'name' '')
$action = Need-Enum 'action' ([string](Arg 'action' '')) @('restart', 'start')
Emit-Ok (Service-Plan $n $action)
