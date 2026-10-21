const integerFormatter = new Intl.NumberFormat('zh-CN');
const decimalFormatter = new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 });

export const formatInteger = value => integerFormatter.format(value);
export const formatDecimal = value => decimalFormatter.format(value);
