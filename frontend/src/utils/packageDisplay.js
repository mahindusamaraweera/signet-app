export function visiblePackageChoices(packages, blockchainEnabled, showLowerPackages) {
  return [...packages]
    .filter(p => !blockchainEnabled || Number(p.price) >= 1000 || showLowerPackages)
    .sort((a, b) => {
      if (blockchainEnabled) {
        const group = Number(Number(a.price) < 1000) - Number(Number(b.price) < 1000)
        if (group) return group
      }
      return Number(a.price) - Number(b.price)
    })
}
