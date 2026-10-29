// Positive-path browser fixtures explicitly accept the current terms and adult declaration.
export async function acceptLegal(page){const box=page.locator('#legal-accept');if(await box.count())await box.check()}
